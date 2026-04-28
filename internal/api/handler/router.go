package handler

import (
	"net/http"

	"github.com/example/go-ai-scheduler/internal/api/middleware"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/tenant"
)

// NewSchedulerRouter wires internal scheduler-facing routes (no auth required).
func NewSchedulerRouter(workerHandler *WorkerHandler, taskRuntimeHandler *TaskRuntimeHandler, eventHandler *EventHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", Health)
	mux.Handle("/metrics", metrics.DefaultRegistry.Handler())
	mux.HandleFunc("/api/v1/workers/register", workerHandler.Register)
	mux.HandleFunc("/api/v1/workers/heartbeat", workerHandler.Heartbeat)
	mux.HandleFunc("/api/v1/task-instances/report", taskRuntimeHandler.Report)
	mux.HandleFunc("POST /api/v1/events/publish", eventHandler.Publish)
	return metrics.Instrument("scheduler", mux)
}

// NewAPIRouter wires external management and query routes with JWT auth and RBAC.
func NewAPIRouter(authHandler *AuthHandler, workerHandler *WorkerHandler, taskHandler *TaskHandler, taskInstanceHandler *TaskInstanceHandler) http.Handler {
	mux := http.NewServeMux()

	// Public endpoints.
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("/healthz", Health)
	mux.Handle("/metrics", metrics.DefaultRegistry.Handler())

	// All /api/v1/* routes require JWT authentication.
	mux.HandleFunc("GET /api/v1/workers", requireAuth("viewer", workerHandler.List))
	mux.HandleFunc("GET /api/v1/workers/", requireAuth("viewer", workerHandler.Get))
	mux.HandleFunc("GET /api/v1/tasks", requireAuth("viewer", taskHandler.List))
	mux.HandleFunc("GET /api/v1/tasks/", requireAuth("viewer", taskHandler.GetOrUpdate))
	mux.HandleFunc("POST /api/v1/tasks", requireAuth("operator", taskHandler.List))       // POST = create
	mux.HandleFunc("PUT /api/v1/tasks/", requireAuth("operator", taskHandler.GetOrUpdate)) // PUT = update
	mux.HandleFunc("DELETE /api/v1/tasks/", requireAuth("admin", taskHandler.GetOrUpdate)) // DELETE
	mux.HandleFunc("POST /api/v1/tasks/{id}/pause", requireAuth("operator", taskHandler.Pause))
	mux.HandleFunc("POST /api/v1/tasks/{id}/resume", requireAuth("operator", taskHandler.Resume))
	mux.HandleFunc("POST /api/v1/tasks/{id}/trigger", requireAuth("operator", taskHandler.Trigger))
	mux.HandleFunc("GET /api/v1/task-instances", requireAuth("viewer", taskInstanceHandler.List))
	mux.HandleFunc("GET /api/v1/task-instances/", requireAuth("viewer", taskInstanceHandler.Get))

	return metrics.Instrument("api", tenant.Middleware(mux))
}

// requireAuth wraps an http.HandlerFunc to require JWT auth and a minimum role.
// Roles: admin > operator > viewer. The minimumRole is the least privileged allowed.
func requireAuth(minimumRole string, next http.HandlerFunc) http.HandlerFunc {
	roleWeight := map[string]int{"admin": 3, "operator": 2, "viewer": 1}

	return func(w http.ResponseWriter, r *http.Request) {
		claims := middleware.GetClaims(r.Context())
		if claims == nil {
			// Try to extract and parse Bearer token from header.
			auth := r.Header.Get("Authorization")
			if auth == "" || !hasBearerPrefix(auth) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"authentication required"}`))
				return
			}
			token := auth[7:] // len("Bearer ") = 7
			parsed, err := middleware.ParseToken(token)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid or expired token"}`))
				return
			}
			claims = parsed
		}

		if roleWeight[claims.Role] < roleWeight[minimumRole] {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"insufficient permissions"}`))
			return
		}

		next(w, r)
	}
}

func hasBearerPrefix(s string) bool {
	return len(s) >= 7 && s[:7] == "Bearer "
}
