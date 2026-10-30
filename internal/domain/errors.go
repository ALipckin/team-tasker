// Package domain contains domain entities, errors, and access rules.
// It has no infrastructure dependencies.
package domain

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrForbidden     = errors.New("forbidden")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
	ErrValidation    = errors.New("validation error")
)
