package stream

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WSWriter streams events over a WebSocket connection.
type WSWriter struct {
	conn   net.Conn
	bufw   *bufio.Writer
	closed bool
}

// NewWSWriter upgrades an HTTP request to a WebSocket connection.
func NewWSWriter(w http.ResponseWriter, r *http.Request) (*WSWriter, error) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, fmt.Errorf("missing Sec-WebSocket-Key header")
	}

	// Compute accept key.
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	acceptKey := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Hijack the connection.
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("server does not support hijacking")
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijack: %w", err)
	}

	// Send upgrade response.
	resp := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", acceptKey)
	if _, err := bufrw.WriteString(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}

	return &WSWriter{conn: conn, bufw: bufrw.Writer}, nil
}

// writeFrame sends a single text frame.
func (s *WSWriter) writeFrame(payload []byte) error {
	if s.closed {
		return nil
	}
	// FIN=1, opcode=1 (text)
	if err := s.conn.SetWriteDeadline(timeNow().Add(30 * time.Second)); err != nil {
		return err
	}
	frame := make([]byte, 0, 2+8+len(payload))
	frame = append(frame, 0x81) // FIN + text opcode
	n := len(payload)
	switch {
	case n <= 125:
		frame = append(frame, byte(n))
	case n <= 65535:
		frame = append(frame, 126)
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(n))
		frame = append(frame, b...)
	default:
		frame = append(frame, 127)
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(n))
		frame = append(frame, b...)
	}
	frame = append(frame, payload...)
	_, err := s.conn.Write(frame)
	return err
}

func (s *WSWriter) send(event string, data any) error {
	msg := map[string]any{"event": event, "data": data}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return s.writeFrame(b)
}

func (s *WSWriter) Text(delta string) error {
	return s.send("text", map[string]string{"delta": delta})
}

func (s *WSWriter) ToolCall(name string, args json.RawMessage) error {
	return s.send("tool_call", map[string]any{"name": name, "args": args})
}

func (s *WSWriter) ToolResult(name string, result any) error {
	return s.send("tool_result", map[string]any{"name": name, "result": result})
}

func (s *WSWriter) Done() error {
	_ = s.send("done", map[string]any{})
	// Send close frame.
	s.closed = true
	closeFrame := []byte{0x88, 0x00} // FIN + close opcode
	s.conn.Write(closeFrame)
	s.conn.Close()
	return nil
}

func (s *WSWriter) Error(err error) error {
	_ = s.send("error", map[string]string{"message": err.Error()})
	s.closed = true
	closeFrame := []byte{0x88, 0x00}
	s.conn.Write(closeFrame)
	s.conn.Close()
	return nil
}

func (s *WSWriter) Action(actionType, title, description string, payload any) error {
	return s.send("action", map[string]any{
		"type": actionType, "title": title, "description": description, "payload": payload,
	})
}

func (s *WSWriter) Event(name string, data any) error {
	return s.send(name, data)
}

// ReadJSON reads and unmarshals a JSON text frame from the WebSocket.
func (s *WSWriter) ReadJSON(v any) error {
	// Read a minimal WebSocket frame.
	s.conn.SetReadDeadline(timeNow().Add(30 * time.Second))
	header := make([]byte, 2)
	if _, err := io.ReadFull(s.conn, header); err != nil {
		return fmt.Errorf("read ws header: %w", err)
	}
	opcode := header[0] & 0x0F
	if opcode == 0x08 { // close frame
		return fmt.Errorf("client closed connection")
	}
	masked := header[1]&0x80 != 0
	length := int64(header[1] & 0x7F)
	switch {
	case length == 126:
		b := make([]byte, 2)
		if _, err := io.ReadFull(s.conn, b); err != nil {
			return err
		}
		length = int64(binary.BigEndian.Uint16(b))
	case length == 127:
		b := make([]byte, 8)
		if _, err := io.ReadFull(s.conn, b); err != nil {
			return err
		}
		length = int64(binary.BigEndian.Uint64(b))
	}
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(s.conn, maskKey[:]); err != nil {
			return err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(s.conn, payload); err != nil {
		return err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return json.Unmarshal(payload, v)
}

// timeNow exists so we can override it in tests.
var timeNow = func() time.Time { return time.Now() }
