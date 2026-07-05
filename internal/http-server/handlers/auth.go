package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"OctoQueue/internal/domain"
	"OctoQueue/internal/lib/logger/sl"
	"OctoQueue/internal/service"

	"github.com/go-chi/chi/v5/middleware"
)

type AuthHandler struct {
	svc service.AuthService
	log *slog.Logger
}

func NewAuthHandler(svc service.AuthService, log *slog.Logger) *AuthHandler {
	return &AuthHandler{svc: svc, log: log}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	const op = "handlers.AuthHandler.Register"
	log := h.log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	
	if err := decode(r, &req); err != nil {
		log.Warn("invalid request body", sl.Err(err))
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}
	
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "email and password are required")
		return
	}

	user, err := h.svc.Register(r.Context(), req.Email, req.Password, domain.RoleUser)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailTaken):
			writeError(w, http.StatusConflict, "EMAIL_TAKEN", "email already registered")
		default:
			log.Error("failed to register user", sl.Err(err))
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to register user")
		}
		return
	}

	log.Info("user registered", slog.String("email", user.Email))

	writeJSON(w, http.StatusCreated, map[string]any{
		"user": map[string]any{
			"id":    user.ID,
			"email": user.Email,
			"role":  user.Role,
		},
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	const op = "handlers.AuthHandler.Login"
	log := h.log.With(slog.String("op", op), slog.String("request_id", middleware.GetReqID(r.Context())))

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	
	if err := decode(r, &req); err != nil {
		log.Warn("invalid request body", sl.Err(err))
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	token, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			log.Warn("login failed: invalid credentials", slog.String("email", req.Email))
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid email or password")
			return
		}
		
		log.Error("login failed: internal server error", sl.Err(err), slog.String("email", req.Email))
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to login")
		return
	}

	log.Info("user logged in", slog.String("email", req.Email))
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}
