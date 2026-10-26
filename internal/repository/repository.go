package repository

import (
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"

	"backend/internal/database"
)

// DB — интерфейс базы данных, который нужен репозиториям.
// Conn возвращает текущую транзакцию из контекста или пул соединений.
type DB interface {
	Conn(ctx context.Context) database.Querier
}

const mysqlErrDuplicateEntry = 1062

func isDuplicate(err error) bool {
	var myErr *mysql.MySQLError
	return errors.As(err, &myErr) && myErr.Number == mysqlErrDuplicateEntry
}
