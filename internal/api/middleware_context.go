package api

import (
	"context"

	"backend/internal/api/middleware"
)

func userIDFromContext(ctx context.Context) int64 {
	return middleware.UserIDFromContext(ctx)
}
