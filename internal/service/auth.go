package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"backend/internal/domain"
)

// AuthUserRepository — интерфейс репозитория пользователей для AuthService.
type AuthUserRepository interface {
	Create(ctx context.Context, u *domain.User) error
	GetByEmail(ctx context.Context, email string) (domain.User, error)
}

// AuthService — интерфейс сервиса авторизации.
// Структура неэкспортируемая — создать объект можно только через NewAuthService.
type AuthService interface {
	Register(ctx context.Context, email, password, name string) (domain.User, error)
	Login(ctx context.Context, email, password string) (string, domain.User, error)
	// ParseToken проверяет JWT и возвращает ID пользователя.
	ParseToken(token string) (int64, error)
}

// authService — сервис авторизации.
type authService struct {
	userRepo AuthUserRepository
	secret   []byte
	ttl      time.Duration
}

// NewAuthService создаёт сервис авторизации.
func NewAuthService(userRepo AuthUserRepository, secret string, ttl time.Duration) AuthService {
	return &authService{
		userRepo: userRepo,
		secret:   []byte(secret),
		ttl:      ttl,
	}
}

const (
	minPasswordLen = 8
	// maxNameLen — длина VARCHAR(255) в символах (utf8mb4).
	maxNameLen = 255
)

// dummyHash сравнивается с паролем, когда пользователь не найден, чтобы Login
// отвечал за одинаковое время. От перебора email это не защищает: Register
// намеренно отвечает 409 на занятый email (часть контракта API), поэтому
// существование аккаунта по-прежнему можно проверить через регистрацию.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)

func (s *authService) Register(ctx context.Context, email, password, name string) (domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)

	if _, err := mail.ParseAddress(email); err != nil {
		return domain.User{}, validationError("invalid email")
	}
	if utf8.RuneCountInString(email) > maxNameLen {
		return domain.User{}, validationError("email is too long")
	}
	if len(password) < minPasswordLen {
		return domain.User{}, validationError(fmt.Sprintf("password must be at least %d characters", minPasswordLen))
	}
	if len(password) > 72 {
		return domain.User{}, validationError("password must be at most 72 bytes")
	}
	if name == "" {
		return domain.User{}, validationError("name is required")
	}
	if utf8.RuneCountInString(name) > maxNameLen {
		return domain.User{}, validationError("name is too long")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}

	u := domain.User{Email: email, PasswordHash: string(hash), Name: name}
	if err = s.userRepo.Create(ctx, &u); err != nil {
		return domain.User{}, err
	}

	slog.InfoContext(ctx, "user registered", "user_id", u.ID)

	return u, nil
}

func (s *authService) Login(ctx context.Context, email, password string) (string, domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	u, err := s.userRepo.GetByEmail(ctx, email)
	if errors.Is(err, domain.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", domain.User{}, fmt.Errorf("invalid credentials: %w", domain.ErrUnauthorized)
	}
	if err != nil {
		return "", domain.User{}, err
	}

	if err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return "", domain.User{}, fmt.Errorf("invalid credentials: %w", domain.ErrUnauthorized)
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(u.ID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
	})

	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", domain.User{}, fmt.Errorf("sign token: %w", err)
	}

	return signed, u, nil
}

func (s *authService) ParseToken(token string) (int64, error) {
	claims := &jwt.RegisteredClaims{}

	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return s.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return 0, fmt.Errorf("parse token: %w", domain.ErrUnauthorized)
	}

	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid token subject: %w", domain.ErrUnauthorized)
	}

	return id, nil
}
