package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ctxKey string

const claimsKey ctxKey = "auth:claims"

// WithClaims stores JWT claims in the context.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// GetClaims retrieves JWT claims from the context. Returns nil if absent.
func GetClaims(ctx context.Context) *Claims {
	claims, ok := ctx.Value(claimsKey).(*Claims)
	if !ok {
		return nil
	}
	return claims
}

// RequireRole is middleware that requires at least one of the given roles.
// Roles use hierarchical weights: admin > operator > viewer.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	roleWeight := map[string]int{"admin": 3, "operator": 2, "viewer": 1}
	// Compute the minimum role weight required.
	minWeight := 0
	for _, r := range roles {
		if w := roleWeight[r]; w > minWeight {
			minWeight = w
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				writeAuthError(w, "authentication required")
				return
			}
			if roleWeight[claims.Role] < minWeight {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "insufficient permissions"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuthenticateOptional adds claims from a Bearer token if present, but does
// not reject requests without a token.
func AuthenticateOptional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" && strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if claims, err := ParseToken(token); err == nil {
				ctx := WithClaims(r.Context(), claims)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}
