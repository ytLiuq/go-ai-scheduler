package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

// jwtSecret is the signing key. Initialised from JWT_SECRET or a random key.
var jwtSecret []byte

func init() {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		jwtSecret = []byte(s)
	}
}

func secret() []byte {
	if jwtSecret != nil {
		return jwtSecret
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	jwtSecret = key
	return jwtSecret
}

// SetJWTSecret overrides the signing key programmatically (for tests).
func SetJWTSecret(secret []byte) {
	jwtSecret = secret
}

// Claims carried in a JWT token.
type Claims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

// SignToken creates a signed JWT token.
func SignToken(claims Claims) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, secret())
	mac.Write([]byte(unsigned))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + sig, nil
}

// ParseToken validates and returns claims from a JWT.
func ParseToken(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, &JWTError{"invalid token format"}
	}
	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret())
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

// Authenticate validates a Bearer token and injects claims into the context.
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
