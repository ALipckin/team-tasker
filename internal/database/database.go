package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Querier — общий набор методов *sql.DB и *sql.Tx.
// Репозитории работают через него и не знают, идёт ли запрос в транзакции.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DB — интерфейс подключения к базе данных.
// Структура неэкспортируемая - создать объект можно только через New.
type DB interface {
	// Conn возвращает транзакцию из контекста, если она есть, иначе пул соединений.
	Conn(ctx context.Context) Querier
	// WithTx выполняет fn в транзакции. Вложенные вызовы переиспользуют внешнюю транзакцию.
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
	Close() error
}

type txKey struct{}

// mysqlDB — обёртка над подключением к базе данных.
type mysqlDB struct {
	db *sql.DB
}

// New открывает подключение к MySQL. Если migrationsPath не пустой — применяет миграции.
func New(dsn, migrationsPath string) (DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err = pingWithRetry(db, 30, time.Second); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if migrationsPath != "" {
		if err = runMigrations(dsn, migrationsPath); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	slog.Info("подключились к БД")

	return &mysqlDB{db: db}, nil
}

func (d *mysqlDB) Conn(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}

	return d.db
}

func (d *mysqlDB) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			slog.ErrorContext(ctx, "rollback failed", "err", rbErr)
		}
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// Close закрывает подключение к базе данных.
func (d *mysqlDB) Close() error {
	slog.Info("подключение к БД закрыто")
	return d.db.Close()
}

// MySQL в docker compose может быть ещё не готов принимать подключения.
func pingWithRetry(db *sql.DB, attempts int, delay time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = db.Ping(); err == nil {
			return nil
		}
		time.Sleep(delay)
	}

	return err
}

// runMigrations применяет миграции через отдельное соединение.
// multiStatements нужен только файлам миграций, поэтому для основного пула он выключен.
func runMigrations(dsn, migrationsPath string) error {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MultiStatements = true

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("open migrations db: %w", err)
	}
	defer db.Close()

	driver, err := migratemysql.WithInstance(db, &migratemysql.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(migrationsPath, "mysql", driver)
	if err != nil {
		return fmt.Errorf("init migrations: %w", err)
	}

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
