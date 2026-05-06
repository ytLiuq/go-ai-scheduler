package handler

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/example/go-ai-scheduler/internal/api/middleware"
	"github.com/example/go-ai-scheduler/internal/pkg/metrics"
	"github.com/example/go-ai-scheduler/internal/tenant"
	"github.com/gin-gonic/gin"
)

// NewSchedulerRouter wires internal scheduler-facing routes (no auth required).
func NewSchedulerRouter(workerHandler *WorkerHandler, taskRuntimeHandler *TaskRuntimeHandler, eventHandler *EventHandler, workerLoadHandler *WorkerLoadHandler) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.GET("/healthz", wrapHTTPHandler(Health))
	engine.GET("/metrics", gin.WrapH(metrics.DefaultRegistry.Handler()))
	engine.POST("/api/v1/workers/register", wrapHTTPHandler(workerHandler.Register))
	engine.POST("/api/v1/workers/heartbeat", wrapHTTPHandler(workerHandler.Heartbeat))
	engine.POST("/api/v1/task-instances/report", wrapHTTPHandler(taskRuntimeHandler.Report))
	engine.POST("/api/v1/events/publish", wrapHTTPHandler(eventHandler.Publish))
	engine.POST("/api/v1/events/receive", wrapHTTPHandler(eventHandler.Publish))
	engine.POST("/api/v1/task-instances/cancel", wrapHTTPHandler(taskRuntimeHandler.Cancel))
	engine.POST("/api/v1/task-instances/ack", wrapHTTPHandler(taskRuntimeHandler.Ack))
	engine.GET("/api/v1/worker-loads", wrapHTTPHandler(workerLoadHandler.List))
	return metrics.Instrument("scheduler", engine)
}

// NewAPIRouter wires external management and query routes with JWT auth and RBAC.
func NewAPIRouter(authHandler *AuthHandler, workerHandler *WorkerHandler, taskHandler *TaskHandler, taskInstanceHandler *TaskInstanceHandler) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Public endpoints.
	engine.POST("/api/auth/login", wrapHTTPHandler(authHandler.Login))
	engine.GET("/healthz", wrapHTTPHandler(Health))
	engine.GET("/metrics", gin.WrapH(metrics.DefaultRegistry.Handler()))

	// All /api/v1/* routes require JWT authentication.
	engine.GET("/api/v1/workers", wrapHTTPHandler(requireAuth("viewer", workerHandler.List)))
	engine.GET("/api/v1/workers/:id", wrapHTTPHandlerWithParams(requireAuth("viewer", workerHandler.Get), "id"))
	engine.GET("/api/v1/tasks", wrapHTTPHandler(requireAuth("viewer", taskHandler.List)))
	engine.GET("/api/v1/tasks/dag", wrapHTTPHandler(requireAuth("viewer", taskHandler.DAG)))
	engine.GET("/api/v1/tasks/:id", wrapHTTPHandlerWithParams(requireAuth("viewer", taskHandler.GetOrUpdate), "id"))
	engine.POST("/api/v1/tasks", wrapHTTPHandler(requireAuth("operator", taskHandler.List)))
	engine.PUT("/api/v1/tasks/:id", wrapHTTPHandlerWithParams(requireAuth("operator", taskHandler.GetOrUpdate), "id"))
	engine.DELETE("/api/v1/tasks/:id", wrapHTTPHandlerWithParams(requireAuth("admin", taskHandler.GetOrUpdate), "id"))
	engine.POST("/api/v1/tasks/:id/pause", wrapHTTPHandlerWithParams(requireAuth("operator", taskHandler.Pause), "id"))
	engine.POST("/api/v1/tasks/:id/resume", wrapHTTPHandlerWithParams(requireAuth("operator", taskHandler.Resume), "id"))
	engine.POST("/api/v1/tasks/:id/trigger", wrapHTTPHandlerWithParams(requireAuth("operator", taskHandler.Trigger), "id"))
	engine.GET("/api/v1/task-instances", wrapHTTPHandler(requireAuth("viewer", taskInstanceHandler.List)))
	engine.GET("/api/v1/task-instances/:id", wrapHTTPHandlerWithParams(requireAuth("viewer", taskInstanceHandler.Get), "id"))

	// AI endpoints proxied to ai-service.
	engine.GET("/api/v1/ai/status", wrapHTTPHandler(requireAuth("viewer", proxyAIHandler(http.MethodGet, "status"))))
	engine.POST("/api/v1/ai/log-analysis/analyze", wrapHTTPHandler(requireAuth("viewer", proxyAIHandler("log-analysis/analyze"))))
	engine.POST("/api/v1/ai/advisor/generate", wrapHTTPHandler(requireAuth("viewer", proxyAIHandler("advisor/generate"))))
	engine.POST("/api/v1/ai/advisor/auto", wrapHTTPHandler(requireAuth("viewer", proxyAIHandler("advisor/auto"))))
	engine.POST("/api/v1/ai/task/predict-duration", wrapHTTPHandler(requireAuth("viewer", proxyAIHandler("task/predict-duration"))))
	engine.POST("/api/v1/ai/trend/analyze", wrapHTTPHandler(requireAuth("viewer", proxyAIHandler("trend/analyze"))))
	engine.POST("/api/v1/ai/task/create", wrapHTTPHandler(requireAuth("operator", proxyAIHandler("task/create"))))
	engine.POST("/api/v1/ai/chat", wrapHTTPHandler(requireAuth("viewer", proxyAIHandler("chat"))))
	engine.GET("/api/v1/ai/chat/ws", wrapHTTPHandler(requireAuth("viewer", proxyWSHandler("chat/ws"))))
	engine.GET("/api/v1/ai/conversations", wrapHTTPHandler(requireAuth("viewer", proxyAIHandler(http.MethodGet, "conversations"))))
	engine.POST("/api/v1/events/receive", wrapHTTPHandler(requireAuth("operator", proxySchedulerHandler("events/publish"))))
	engine.GET("/api/v1/ai/conversations/*rest", wrapHTTPHandler(requireAuth("viewer", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/ai/conversations/")
		proxyAIHandler(http.MethodGet, "conversations/"+rest)(w, r)
	})))

	// Serve web console static files.
	engine.NoRoute(gin.WrapH(http.FileServer(http.Dir("web"))))

	return metrics.Instrument("api", tenant.Middleware(engine))
}

func wrapHTTPHandler(next http.HandlerFunc) gin.HandlerFunc {
	return gin.WrapF(next)
}

func wrapHTTPHandlerWithParams(next http.HandlerFunc, paramNames ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := c.Request.Clone(c.Request.Context())
		for _, name := range paramNames {
			req.SetPathValue(name, c.Param(name))
		}
		next(c.Writer, req)
	}
}

// requireAuth wraps an http.HandlerFunc to require JWT auth and a minimum role.
// Roles: admin > operator > viewer. The minimumRole is the least privileged allowed.
func requireAuth(minimumRole string, next http.HandlerFunc) http.HandlerFunc {
	roleWeight := map[string]int{"admin": 3, "operator": 2, "viewer": 1}

	return func(w http.ResponseWriter, r *http.Request) {
		claims := middleware.GetClaims(r.Context())
		if claims == nil {
			// Try token from Bearer header or query string (for WebSocket auth).
			token := ""
			auth := r.Header.Get("Authorization")
			if auth != "" && hasBearerPrefix(auth) {
				token = auth[7:] // len("Bearer ") = 7
			} else if qt := r.URL.Query().Get("token"); qt != "" {
				token = qt
			}
			if token == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"authentication required"}`))
				return
			}
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

		// Inject tenant ID into context.
		if claims.TenantID > 0 {
			ctx := tenant.WithTenant(r.Context(), claims.TenantID)
			r = r.WithContext(ctx)
		}
		next(w, r)
	}
}

func hasBearerPrefix(s string) bool {
	return len(s) >= 7 && s[:7] == "Bearer "
}

// proxyAIHandler returns a handler that reverse-proxies to the AI service.
func proxyAIHandler(methodOrPath string, maybePath ...string) http.HandlerFunc {
	aiServiceURL := os.Getenv("AI_SERVICE_URL")
	if aiServiceURL == "" {
		aiServiceURL = "http://127.0.0.1:8083"
	}
	method := http.MethodPost
	path := methodOrPath
	if len(maybePath) > 0 {
		method = methodOrPath
		path = maybePath[0]
	}
	target := strings.TrimRight(aiServiceURL, "/") + "/api/v1/" + path

	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		proxyReq, err := http.NewRequestWithContext(r.Context(), method, target, io.NopCloser(strings.NewReader(string(body))))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"proxy error"}`))
			return
		}
		if method != http.MethodGet {
			proxyReq.Header.Set("Content-Type", "application/json")
		}
		// Forward tracing header.
		if traceID := r.Header.Get("X-Trace-ID"); traceID != "" {
			proxyReq.Header.Set("X-Trace-ID", traceID)
		}

		resp, err := http.DefaultClient.Do(proxyReq)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"ai service unreachable"}`))
			return
		}
		defer resp.Body.Close()

		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}
}

// proxySchedulerHandler returns a handler that reverse-proxies to the scheduler service.
func proxySchedulerHandler(path string) http.HandlerFunc {
	baseURL := os.Getenv("SCHEDULER_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8081"
	}
	target := strings.TrimRight(baseURL, "/") + "/api/v1/" + path
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, io.NopCloser(strings.NewReader(string(body))))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "proxy error"})
			return
		}
		proxyReq.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(proxyReq)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "scheduler unreachable"})
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// proxyWSHandler tunnels WebSocket connections to the AI service.
func proxyWSHandler(path string) http.HandlerFunc {
	aiServiceURL := os.Getenv("AI_SERVICE_URL")
	if aiServiceURL == "" {
		aiServiceURL = "http://127.0.0.1:8083"
	}

	return func(w http.ResponseWriter, r *http.Request) {
		backendHost := strings.TrimPrefix(strings.TrimPrefix(aiServiceURL, "http://"), "https://")
		backendConn, err := net.Dial("tcp", backendHost)
		if err != nil {
			http.Error(w, "backend unreachable", http.StatusBadGateway)
			return
		}
		defer backendConn.Close()

		reqStr := fmt.Sprintf("GET /api/v1/%s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
			path, backendHost, r.Header.Get("Sec-WebSocket-Key"))
		if _, err := backendConn.Write([]byte(reqStr)); err != nil {
			http.Error(w, "backend write failed", http.StatusBadGateway)
			return
		}

		buf := make([]byte, 4096)
		n, err := backendConn.Read(buf)
		if err != nil || !strings.Contains(string(buf[:n]), "101") {
			http.Error(w, "backend upgrade failed", http.StatusBadGateway)
			return
		}

		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack not supported", http.StatusInternalServerError)
			return
		}
		clientConn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		defer clientConn.Close()

		clientConn.Write(buf[:n])

		done := make(chan struct{}, 2)
		go func() { io.Copy(backendConn, clientConn); done <- struct{}{} }()
		go func() { io.Copy(clientConn, backendConn); done <- struct{}{} }()
		<-done
	}
}
