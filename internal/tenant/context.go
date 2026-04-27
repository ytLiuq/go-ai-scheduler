package tenant

import "context"

type key struct{}

// WithTenant stores a tenant ID in the context.
func WithTenant(ctx context.Context, tenantID int64) context.Context {
	return context.WithValue(ctx, key{}, tenantID)
}

// FromContext returns the tenant ID stored in the context, or 0 if none.
func FromContext(ctx context.Context) int64 {
	id, _ := ctx.Value(key{}).(int64)
	return id
}
