package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"OctoQueue/internal/domain"
	"OctoQueue/internal/lib/logger/sl"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func Auth(log *slog.Logger, secret string) func(http.Handler) http.Handler {
	const op = "middleware.auth.Auth"
	
	if secret == "" {
		panic("auth: JWT secret must not be empty")
    }
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))

			authHeader := r.Header.Get("Authorization")
			parts := strings.Fields(authHeader)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				logger.Error("Error authHeader")
				writeUnauthorized(w)
				return
			}
			tokenStr := parts[1]
			claims := jwt.MapClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims,
				func(t *jwt.Token) (interface{}, error) {
					if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
						logger.Error("unexpected signing method", t.Header["alg"])
						return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
					}
					return []byte(secret), nil
				})
			if err != nil || !token.Valid {
				logger.Error("Invalid token", sl.Err(err))
				writeUnauthorized(w)
				return
			}
			userIDStr, ok := claims["user_id"].(string)
			if !ok {
				logger.Error("User ID not found in claims")
				writeUnauthorized(w)
				return
			}
			userID, err := uuid.Parse(userIDStr)
			if err != nil {
				logger.Error("failed to parse user ID: %v", sl.Err(err))
				writeUnauthorized(w)
				return
			}
			role, _ := claims["role"].(string)
			ctx := context.WithValue(r.Context(), domain.CtxUserID, userID)
			ctx = context.WithValue(ctx, domain.CtxRole, domain.Role(role))
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

var ErrUserIDNotFound = errors.New("user id not found in context")

func GetUserID(ctx context.Context) (uuid.UUID, error) {
	id, ok := ctx.Value(domain.CtxUserID).(uuid.UUID)
	if !ok {
		return uuid.Nil, ErrUserIDNotFound
	}
	return id, nil
}

func getRole(ctx context.Context) domain.Role {
	role, _ := ctx.Value(domain.CtxRole).(domain.Role)
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
