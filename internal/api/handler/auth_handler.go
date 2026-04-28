package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/example/go-ai-scheduler/internal/api/middleware"
)

// AuthHandler exposes authentication endpoints.
type AuthHandler struct{}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	Role      string `json:"role"`
	ExpiresIn int64  `json:"expires_in"`
}

// Login handles POST /api/auth/login. Simple demo-only credentials.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	// Demo hardcoded users. In production, replace with DB lookup.
	role := authenticateUser(req.Username, req.Password)
	if role == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
		return
	}

	now := time.Now().Unix()
	claims := middleware.Claims{
		Sub:  req.Username,
		Role: role,
		Iat:  now,
		Exp:  now + 3600, // 1 hour
	}
	token, err := middleware.SignToken(claims)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "token generation failed"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(loginResponse{
		Token:     token,
		Role:      role,
		ExpiresIn: 3600,
	})
}

func authenticateUser(username, password string) string {
	// Demo users. Replace with real authentication in production.
	switch username {
	case "admin":
		if password == "admin123" {
			return "admin"
		}
	case "operator":
		if password == "operator123" {
			return "operator"
		}
	case "viewer":
		if password == "viewer123" {
			return "viewer"
		}
	}
	return ""
}
