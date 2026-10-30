package middleware

import (
	"context"
	"net/http"
)

type userIDContextKey struct{}

type userIDSetter interface {
	SetUserID(int64)
}

func SetUserID(w http.ResponseWriter, userID int64) {
	if setter, ok := w.(userIDSetter); ok {
		setter.SetUserID(userID)
	}
}

func ContextWithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

func UserIDFromContext(ctx context.Context) int64 {
	userID, _ := ctx.Value(userIDContextKey{}).(int64)
	return userID
}
