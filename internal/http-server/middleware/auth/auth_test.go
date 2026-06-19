package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"OctoQueue/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testSecret = "test-secret-key"

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func makeToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func validClaims(userID string) jwt.MapClaims {
	return jwt.MapClaims{
		"user_id": userID,
		"role":    "admin",
		"exp":     time.Now().Add(time.Hour).Unix(),
	}
}

func TestAuth_NoAuthHeader(t *testing.T) {
	handler := Auth(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_MalformedHeader(t *testing.T) {
	cases := []string{
		"Bearer",             // нет токена после схемы
		"Basic abc123",       // не Bearer-схема
		"BearerXabc123",      // нет пробела между схемой и токеном
		"Bearer token extra", // лишний третий компонент
	}

	for _, header := range cases {
		t.Run(header, func(t *testing.T) {
			handler := Auth(testSecret)(okHandler())
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", header)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for %q, got %d", header, rec.Code)
			}
		})
	}
}

func TestAuth_InvalidSignature(t *testing.T) {
	token := makeToken(t, "wrong-secret", validClaims(uuid.NewString()))
	handler := Auth(testSecret)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_WrongSigningMethod(t *testing.T) {
	claims := validClaims(uuid.NewString())
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to sign: %v", err)
	}

	handler := Auth(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_ExpiredToken(t *testing.T) {
	claims := jwt.MapClaims{
		"user_id": uuid.NewString(),
		"role":    "admin",
		"exp":     time.Now().Add(-time.Hour).Unix(),
	}
	token := makeToken(t, testSecret, claims)

	handler := Auth(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rec.Code)
	}
}

func TestAuth_MissingUserID(t *testing.T) {
	claims := jwt.MapClaims{
		"role": "admin",
		"exp":  time.Now().Add(time.Hour).Unix(),
		// user_id намеренно отсутствует
	}
	token := makeToken(t, testSecret, claims)

	handler := Auth(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_InvalidUserIDFormat(t *testing.T) {
	token := makeToken(t, testSecret, validClaims("not-a-uuid"))

	handler := Auth(testSecret)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAuth_Success(t *testing.T) {
	expectedID := uuid.New()
	token := makeToken(t, testSecret, validClaims(expectedID.String()))

	var gotUserID uuid.UUID
	var gotRole domain.Role
	var getErr error

	captureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, getErr = GetUserID(r.Context())
		gotRole = getRole(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(testSecret)(captureHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if getErr != nil {
		t.Errorf("unexpected error from GetUserID: %v", getErr)
	}
	if gotUserID != expectedID {
		t.Errorf("expected user_id %s, got %s", expectedID, gotUserID)
	}
	if gotRole != domain.Role("admin") {
		t.Errorf("expected role admin, got %s", gotRole)
	}
}

func TestAuth_MissingRole_StillSucceeds(t *testing.T) {
	claims := jwt.MapClaims{
		"user_id": uuid.NewString(),
		"exp":     time.Now().Add(time.Hour).Unix(),
		// role намеренно отсутствует
	}
	token := makeToken(t, testSecret, claims)

	var gotRole domain.Role
	captureHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRole = getRole(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(testSecret)(captureHandler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotRole != domain.Role("") {
		t.Errorf("expected empty role, got %q", gotRole)
	}
}

func TestAuth_PanicsOnEmptySecret(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty secret, got none")
		}
	}()
	Auth("")
}

func TestRequireRole_Allowed(t *testing.T) {
	handler := RequireRole(domain.Role("admin"))(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.Role("admin"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireRole_Forbidden(t *testing.T) {
	handler := RequireRole(domain.Role("admin"))(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), domain.CtxRole, domain.Role("user"))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireRole_NoRoleInContext(t *testing.T) {
	handler := RequireRole(domain.Role("admin"))(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil) // роль не задана
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestGetUserID_ReturnsErrorWithoutContext(t *testing.T) {
	_, err := GetUserID(context.Background())
	if !errors.Is(err, ErrUserIDNotFound) {
		t.Errorf("expected ErrUserIDNotFound, got %v", err)
	}
}

func TestGetUserID_Success(t *testing.T) {
	expected := uuid.New()
	ctx := context.WithValue(context.Background(), domain.CtxUserID, expected)

	id, err := GetUserID(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != expected {
		t.Errorf("expected %s, got %s", expected, id)
	}
}