package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"backend/internal/domain"
)

// UserRepo — интерфейс репозитория пользователей.
// Структура неэкспортируемая — создать объект можно только через NewUserRepo.
type UserRepo interface {
	Create(ctx context.Context, u *domain.User) error
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	GetByID(ctx context.Context, id int64) (domain.User, error)
}

// userRepo — репозиторий пользователей.
type userRepo struct {
	db DB
}

// NewUserRepo создаёт репозиторий пользователей.
func NewUserRepo(db DB) UserRepo {
	return &userRepo{db: db}
}

func (r *userRepo) Create(ctx context.Context, u *domain.User) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO users (email, password_hash, name) VALUES (?, ?, ?)`,
		u.Email, u.PasswordHash, u.Name,
	)
	if err != nil {
		if isDuplicate(err) {
			return fmt.Errorf("user with email %s: %w", u.Email, domain.ErrAlreadyExists)
		}
		return fmt.Errorf("insert user: %w", err)
	}

	if u.ID, err = res.LastInsertId(); err != nil {
		return fmt.Errorf("user last insert id: %w", err)
	}

	created, err := r.GetByID(ctx, u.ID)
	if err != nil {
		return err
	}
	u.CreatedAt = created.CreatedAt

	return nil
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.getOne(ctx, `SELECT id, email, password_hash, name, created_at FROM users WHERE email = ?`, email)
}

func (r *userRepo) GetByID(ctx context.Context, id int64) (domain.User, error) {
	return r.getOne(ctx, `SELECT id, email, password_hash, name, created_at FROM users WHERE id = ?`, id)
}

func (r *userRepo) getOne(ctx context.Context, query string, arg any) (domain.User, error) {
	var u domain.User

	err := r.db.Conn(ctx).QueryRowContext(ctx, query, arg).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return u, fmt.Errorf("user: %w", domain.ErrNotFound)
	}
	if err != nil {
		return u, fmt.Errorf("select user: %w", err)
	}

	return u, nil
}
