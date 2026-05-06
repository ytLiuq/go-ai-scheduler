package ai

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/go-ai-scheduler/internal/ai/adapter"
	"github.com/example/go-ai-scheduler/internal/ai/agent"
	"github.com/example/go-ai-scheduler/internal/ai/memory"
	"github.com/example/go-ai-scheduler/internal/ai/stream"
	"github.com/example/go-ai-scheduler/internal/ai/tools"
)

type chatRequest struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id,omitempty"`
}

func handleChat(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, registry *tools.Registry, store *memory.Store) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r.Method)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}

	// Create event writer: WebSocket if client requests upgrade, SSE otherwise.
	var sw stream.EventWriter
	if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
		ws, err := stream.NewWSWriter(w, r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "websocket upgrade failed: " + err.Error()})
			return
		}
		sw = ws
	} else {
		sse, err := stream.NewSSEWriter(w)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
			return
		}
		sw = sse
	}

	// Load or create conversation.
	convID := req.ConversationID
	var history []adapter.Message
	if convID != "" && store != nil {
		msgs, _ := store.GetHistory(r.Context(), convID, 50)
		for _, m := range msgs {
			history = append(history, adapter.Message{
				Role:    m.Role,
				Content: m.Content,
			})
		}
	}
	if convID == "" && store != nil {
		conv, err := store.CreateConversation(r.Context(), firstLine(req.Message, 50))
		if err != nil {
			slog.Warn("create conversation failed", "error", err)
		} else {
			convID = conv.ID
		}
	}

	// Save user message.
	if convID != "" && store != nil {
		_ = store.AddMessage(r.Context(), convID, "user", req.Message, "")
	}

	// Run agent.
	result, runErr := agent.Run(r.Context(), llm, registry, agent.SystemPrompt, history, req.Message, sw)
	if runErr != nil {
		slog.Warn("agent run error", "error", runErr)
		sw.Error(runErr)
		return
	}

	// Save assistant response.
	if convID != "" && store != nil && result.Content != "" {
		toolCallsJSON := "null"
		if len(result.ToolCalls) > 0 {
			toolCallsJSON = fmt.Sprintf("%q", result.ToolCalls)
		}
		_ = store.AddMessage(r.Context(), convID, "assistant", result.Content, toolCallsJSON)
	}

	// Send conversation ID.
	if convID != "" {
		sw.Event("conversation_id", map[string]string{"id": convID})
	}
}

func listConversations(w http.ResponseWriter, r *http.Request, store *memory.Store) {
	if store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"conversations": []any{}})
		return
	}
	convs, err := store.ListConversations(r.Context(), 20)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": convs})
}

// handleChatWS handles WebSocket chat connections.
func handleChatWS(w http.ResponseWriter, r *http.Request, llm *adapter.LLMAdapter, registry *tools.Registry, store *memory.Store) {
	ws, err := stream.NewWSWriter(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "websocket upgrade failed: " + err.Error()})
		return
	}

	var req chatRequest
	if err := ws.ReadJSON(&req); err != nil {
		ws.Error(fmt.Errorf("invalid message: %w", err))
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		ws.Error(fmt.Errorf("message is required"))
		return
	}

	convID := req.ConversationID
	var history []adapter.Message
	if convID != "" && store != nil {
		msgs, _ := store.GetHistory(r.Context(), convID, 50)
		for _, m := range msgs {
			history = append(history, adapter.Message{Role: m.Role, Content: m.Content})
		}
	}
	if convID == "" && store != nil {
		conv, err := store.CreateConversation(r.Context(), firstLine(req.Message, 50))
		if err != nil {
			slog.Warn("create conversation failed", "error", err)
		} else {
			convID = conv.ID
		}
	}
	if convID != "" && store != nil {
		_ = store.AddMessage(r.Context(), convID, "user", req.Message, "")
	}

	result, runErr := agent.Run(r.Context(), llm, registry, agent.SystemPrompt, history, req.Message, ws)
	if runErr != nil {
		slog.Warn("agent run error", "error", runErr)
		return
	}

	if convID != "" && store != nil && result.Content != "" {
		toolCallsJSON := "null"
		if len(result.ToolCalls) > 0 {
			toolCallsJSON = fmt.Sprintf("%q", result.ToolCalls)
		}
		_ = store.AddMessage(r.Context(), convID, "assistant", result.Content, toolCallsJSON)
	}
	if convID != "" {
		ws.Event("conversation_id", map[string]string{"id": convID})
	}
}

func getConversationMessages(w http.ResponseWriter, r *http.Request, store *memory.Store) {
	if store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"messages": []any{}})
		return
	}
	convID := r.PathValue("id")
	if convID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "conversation id is required"})
		return
	}
	msgs, err := store.GetHistory(r.Context(), convID, 200)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	result := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		result = append(result, map[string]any{
			"id": m.ID, "role": m.Role, "content": m.Content, "created_at": m.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": result})
}

func firstLine(s string, maxLen int) string {
	// Remove newlines.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
