package middleware

import (
	"net/http"
	"strings"
)

type TokenParser func(string) (int64, error)

func Auth(parseToken TokenParser, writeError ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized") //nolint:misspell // Preserve the established API error code.
				return
			}

			userID, err := parseToken(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized") //nolint:misspell // Preserve the established API error code.
				return
			}

			SetUserID(w, userID)
			next.ServeHTTP(w, r.WithContext(ContextWithUserID(r.Context(), userID)))
		})
	}
}
