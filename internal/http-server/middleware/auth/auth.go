package auth

import (
	"context"
	"fmt"
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
			parts := strings.Fields(authHeader)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeUnauthorized(w)
				return
			}
			tokenStr := parts[1]
			claims := jwt.MapClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims,
				func(t *jwt.Token) (interface{}, error) {
					if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
						return nil, jwt.ErrSignatureInvalid
					}
					return []byte(secret), nil
				})
			if err != nil || !token.Valid {
				fmt.Println("Invalid token") // Debugging log
				writeUnauthorized(w)
				return
			}
			userIDStr, ok := claims["user_id"].(string)
			if !ok {
				fmt.Println("User ID not found in claims") // Debugging log
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
