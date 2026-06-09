package auth

import (
	"context"
	"net/http"
	"strings"

	"OctoQueue/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const (
	CtxUserID contextKey = "user_id"
	CtxRole   contextKey = "role"
)

func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				writeUnauthorized(w)
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims := jwt.MapClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				writeUnauthorized(w)
				return
			}
			userIDStr, ok := claims["user_id"].(string)
			if !ok {
				writeUnauthorized(w)
				return
			}
			userID, err := uuid.Parse(userIDStr)
			if err != nil {
				writeUnauthorized(w)
				return
			}
			role, _ := claims["role"].(string)
			ctx := context.WithValue(r.Context(), CtxUserID, userID)
			ctx = context.WithValue(ctx, CtxRole, domain.Role(role))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRole(role domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if getRole(r.Context()) != role {
				writeForbidden(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func GetUserID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(CtxUserID).(uuid.UUID)
	return id
}

func getRole(ctx context.Context) domain.Role {
	role, _ := ctx.Value(CtxRole).(domain.Role)
	return role
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"unauthorized"}}`))
}

func writeForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"error":{"code":"FORBIDDEN","message":"forbidden"}}`))
}
