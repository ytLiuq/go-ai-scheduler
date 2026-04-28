package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// JWT secret key. In production, this should come from configuration.
var jwtSecret = []byte("go-ai-scheduler-jwt-secret-change-me")

// SetJWTSecret overrides the default JWT signing key.
func SetJWTSecret(secret []byte) {
	jwtSecret = secret
}

// Claims carried in a JWT token.
type Claims struct {
	Sub  string `json:"sub"`  // user ID
	Role string `json:"role"` // admin, operator, viewer
	Iat  int64  `json:"iat"`  // issued at
	Exp  int64  `json:"exp"`  // expiration
}

// SignToken creates a signed JWT token for the given claims.
func SignToken(claims Claims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig, nil
}

// ParseToken validates a JWT token and returns its claims.
func ParseToken(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, &JWTError{"invalid token format"}
	}
	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, jwtSecret)
	mac.Write([]byte(unsigned))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, &JWTError{"invalid signature"}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, &JWTError{"invalid payload encoding"}
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, &JWTError{"invalid claims"}
	}
	if time.Now().Unix() > claims.Exp {
		return nil, &JWTError{"token expired"}
	}
	return &claims, nil
}

// JWTError is returned for token validation failures.
type JWTError struct {
	Message string
}

func (e *JWTError) Error() string {
	return "jwt: " + e.Message
}

// Authenticate is an HTTP middleware that validates a Bearer token and injects
// claims into the request context.
func Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			writeAuthError(w, "missing or malformed authorization header")
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		claims, err := ParseToken(token)
		if err != nil {
			writeAuthError(w, err.Error())
			return
		}
		ctx := WithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeAuthError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
