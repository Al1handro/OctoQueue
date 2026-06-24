package handlers

import (
	"errors"
	"net/http"

	"OctoQueue/internal/domain"
	"OctoQueue/internal/service"
)

type AuthHandler struct {
	svc service.AuthService
}

func NewAuthHandler(svc service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	const op = "handlers.AuthHandler.Register"
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, 400, "INVALID_REQUEST", "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, 400, "INVALID_REQUEST", "email and password are required")
		return
	}
	user, err := h.svc.Register(r.Context(), req.Email, req.Password, domain.RoleUser)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailTaken):
			writeError(w, http.StatusConflict, "EMAIL_TAKEN", "email already registered")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to register user")
		}
		return
	}
	writeJSON(w, 201, map[string]any{"user": user})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	const op = "handlers.AuthHandler.Login"

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, 400, "INVALID_REQUEST", "invalid request body")
		// domainErrToHTTP()
		return
	}
	token, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid email or password")
		return
	}
	writeJSON(w, 200, map[string]string{"token": token})
}
