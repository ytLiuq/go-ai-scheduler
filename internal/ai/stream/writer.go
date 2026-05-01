package stream

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Writer streams Server-Sent Events to an http.ResponseWriter.
type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
	started bool
}

// NewWriter creates an SSE writer. It validates that the http.ResponseWriter
// supports flushing and sets the Content-Type header.
func NewWriter(w http.ResponseWriter) (*Writer, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return &Writer{w: w, flusher: flusher}, nil
}

// Text sends a text delta event (streaming token).
func (s *Writer) Text(delta string) error {
	return s.send("text", map[string]string{"delta": delta})
}

// ToolCall sends a tool_call event (LLM is about to call a function).
func (s *Writer) ToolCall(name string, args json.RawMessage) error {
	return s.send("tool_call", map[string]any{
		"name": name,
		"args": args,
	})
}

// ToolResult sends the result of a tool execution.
func (s *Writer) ToolResult(name string, result any) error {
	return s.send("tool_result", map[string]any{
		"name":   name,
		"result": result,
	})
}

// Done signals the end of the stream.
func (s *Writer) Done() error {
	return s.send("done", map[string]any{})
}

// Error sends an error event.
func (s *Writer) Error(err error) error {
	return s.send("error", map[string]string{"message": err.Error()})
}

// Action prompts the user to approve or take an action.
func (s *Writer) Action(actionType, title, description string, payload any) error {
	return s.send("action", map[string]any{
		"type":        actionType,
		"title":       title,
		"description": description,
		"payload":     payload,
	})
}

// Event sends a custom named event with arbitrary data.
func (s *Writer) Event(name string, data any) error {
	return s.send(name, data)
}

func (s *Writer) send(event string, data any) error {
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
