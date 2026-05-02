package memory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Store persists conversation history.
type Store struct {
	db *sql.DB
}

// NewStore creates a conversation memory store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Message represents a single chat message in a conversation.
type Message struct {
	ID             int64
	ConversationID string
	Role           string // user, assistant, tool
	Content        string
	ToolCallsJSON  string
	CreatedAt      time.Time
}

// Conversation metadata.
type Conversation struct {
	ID           string
	Title        string
	UserID       string
	MessageCount int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CreateConversation starts a new conversation thread.
func (s *Store) CreateConversation(ctx context.Context, title string) (*Conversation, error) {
	id := newID()
	if title == "" {
		title = "新对话"
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO ai_conversation (id, title) VALUES (?, ?)", id, title)
	if err != nil {
		return nil, fmt.Errorf("create conversation: %w", err)
	}
	return &Conversation{ID: id, Title: title, CreatedAt: time.Now()}, nil
}

// AddMessage appends a message to a conversation.
func (s *Store) AddMessage(ctx context.Context, convID, role, content, toolCallsJSON string) error {
	if toolCallsJSON == "" {
		toolCallsJSON = "null"
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO ai_message (conversation_id, role, content, tool_calls_json) VALUES (?, ?, ?, ?)",
		convID, role, content, toolCallsJSON)
	if err != nil {
		return fmt.Errorf("add message: %w", err)
	}
	_, _ = s.db.ExecContext(ctx,
		"UPDATE ai_conversation SET message_count = message_count + 1, updated_at = NOW() WHERE id = ?", convID)
	return nil
}

// GetHistory returns the message history for a conversation.
func (s *Store) GetHistory(ctx context.Context, convID string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, conversation_id, role, content, COALESCE(tool_calls_json, ''), created_at FROM ai_message WHERE conversation_id = ? ORDER BY id ASC LIMIT ?",
		convID, limit)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.ToolCallsJSON, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// ListConversations returns recent conversation threads.
func (s *Store) ListConversations(ctx context.Context, limit int) ([]Conversation, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, title, user_id, message_count, created_at, updated_at FROM ai_conversation ORDER BY updated_at DESC LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var convs []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.UserID, &c.MessageCount, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		convs = append(convs, c)
	}
	return convs, rows.Err()
}

// DeleteConversation removes a conversation and its messages.
func (s *Store) DeleteConversation(ctx context.Context, convID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM ai_message WHERE conversation_id = ?", convID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM ai_conversation WHERE id = ?", convID)
	return err
}

// UpdateTitle changes the conversation title.
func (s *Store) UpdateTitle(ctx context.Context, convID, title string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE ai_conversation SET title = ? WHERE id = ?", title, convID)
	return err
}

// DeleteOldConversations removes conversations not updated since the given time.
func (s *Store) DeleteOldConversations(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM ai_message WHERE conversation_id IN (SELECT id FROM ai_conversation WHERE updated_at < ?)`, before)
	if err != nil {
		return 0, fmt.Errorf("delete old messages: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM ai_conversation WHERE updated_at < ?`, before)
	if err != nil {
		return 0, fmt.Errorf("delete old conversations: %w", err)
	}
	n, _ := result.RowsAffected()
	return n, nil
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
