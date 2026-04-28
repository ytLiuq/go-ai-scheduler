package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSignAndParseToken(t *testing.T) {
	SetJWTSecret([]byte("test-secret"))

	claims := Claims{
		Sub:  "admin",
		Role: "admin",
		Iat:  time.Now().Unix(),
		Exp:  time.Now().Add(3600).Unix(),
	}
	token, err := SignToken(claims)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	parsed, err := ParseToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if parsed.Sub != "admin" || parsed.Role != "admin" {
		t.Fatalf("unexpected claims: sub=%s role=%s", parsed.Sub, parsed.Role)
	}
}

func TestParseTokenInvalidSignature(t *testing.T) {
	SetJWTSecret([]byte("test-secret"))

	claims := Claims{Sub: "x", Role: "viewer", Iat: time.Now().Unix(), Exp: time.Now().Add(3600).Unix()}
	token, _ := SignToken(claims)

	// Tamper with secret.
	SetJWTSecret([]byte("wrong-secret"))
	_, err := ParseToken(token)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestParseTokenExpired(t *testing.T) {
	SetJWTSecret([]byte("test-secret"))

	// Use fixed past timestamps to guarantee expiry.
	claims := Claims{Sub: "x", Role: "viewer", Iat: 1000000000, Exp: 1000003600}
	token, _ := SignToken(claims)

	_, err := ParseToken(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestParseTokenBadFormat(t *testing.T) {
	SetJWTSecret([]byte("test-secret"))

	_, err := ParseToken("not.a.token")
	if err == nil {
		t.Fatal("expected error for bad format")
	}
}

func TestAuthenticateMiddlewareMissingHeader(t *testing.T) {
	SetJWTSecret([]byte("test-secret"))

	handler := Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthenticateMiddlewareValidToken(t *testing.T) {
	SetJWTSecret([]byte("test-secret"))

	claims := Claims{Sub: "admin", Role: "admin", Iat: time.Now().Unix(), Exp: time.Now().Add(3600).Unix()}
	token, _ := SignToken(claims)

	var capturedClaims *Claims
	handler := Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if capturedClaims == nil || capturedClaims.Role != "admin" {
		t.Fatal("claims not injected into context")
	}
}

func TestRequireRoleMiddleware(t *testing.T) {
	SetJWTSecret([]byte("test-secret"))

	tests := []struct {
		name         string
		userRole     string
		requiredRole string
		wantStatus   int
	}{
		{"admin can access admin", "admin", "admin", http.StatusOK},
		{"operator cannot access admin", "operator", "admin", http.StatusForbidden},
		{"viewer cannot access operator", "viewer", "operator", http.StatusForbidden},
		{"admin can access viewer", "admin", "viewer", http.StatusOK},
		{"operator can access viewer", "operator", "viewer", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := Claims{Sub: "u", Role: tt.userRole, Iat: time.Now().Unix(), Exp: time.Now().Add(3600).Unix()}
			token, _ := SignToken(claims)

			handler := Authenticate(RequireRole(tt.requiredRole)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestAuthenticateOptional(t *testing.T) {
	SetJWTSecret([]byte("test-secret"))

	var capturedClaims *Claims
	handler := AuthenticateOptional(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedClaims = GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Without token.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || capturedClaims != nil {
		t.Fatal("expected no claims without token")
	}

	// With token.
	claims := Claims{Sub: "u", Role: "viewer", Iat: time.Now().Unix(), Exp: time.Now().Add(3600).Unix()}
	token, _ := SignToken(claims)
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || capturedClaims == nil {
		t.Fatal("expected claims with valid token")
	}
}
