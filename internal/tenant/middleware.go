package tenant

import (
	"net/http"
	"strconv"
)

// Middleware extracts X-Tenant-ID from the request header and stores it in the context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tidStr := r.Header.Get("X-Tenant-ID")
		if tidStr != "" {
			if tid, err := strconv.ParseInt(tidStr, 10, 64); err == nil && tid > 0 {
				ctx := WithTenant(r.Context(), tid)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}
