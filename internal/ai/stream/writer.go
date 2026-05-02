package stream

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// EventWriter is the interface for streaming chat events to a client.
// Both SSE and WebSocket transports implement this.
type EventWriter interface {
	Text(delta string) error
	ToolCall(name string, args json.RawMessage) error
	ToolResult(name string, result any) error
	Done() error
	Error(err error) error
	Action(actionType, title, description string, payload any) error
	Event(name string, data any) error
}

// SSEWriter streams Server-Sent Events to an http.ResponseWriter.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	started bool
}

// NewSSEWriter creates an SSE writer.
func NewSSEWriter(w http.ResponseWriter) (*SSEWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return &SSEWriter{w: w, flusher: flusher}, nil
}

func (s *SSEWriter) Text(delta string) error {
	return s.send("text", map[string]string{"delta": delta})
}

func (s *SSEWriter) ToolCall(name string, args json.RawMessage) error {
	return s.send("tool_call", map[string]any{"name": name, "args": args})
}

func (s *SSEWriter) ToolResult(name string, result any) error {
	return s.send("tool_result", map[string]any{"name": name, "result": result})
}

func (s *SSEWriter) Done() error {
	return s.send("done", map[string]any{})
}

func (s *SSEWriter) Error(err error) error {
	return s.send("error", map[string]string{"message": err.Error()})
}

func (s *SSEWriter) Action(actionType, title, description string, payload any) error {
	return s.send("action", map[string]any{
		"type": actionType, "title": title, "description": description, "payload": payload,
	})
}

func (s *SSEWriter) Event(name string, data any) error {
	return s.send(name, data)
}

func (s *SSEWriter) send(event string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if !s.started {
		s.started = true
	}
	_, err = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, jsonData)
	if err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}
