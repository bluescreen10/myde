// Package protocol implements the framed JSON transport shared by LSP and DAP.
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Message is a JSON-RPC or debug adapter message.
type Message struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`

	Sequence        int             `json:"seq,omitempty"`
	Type            string          `json:"type,omitempty"`
	RequestSequence int             `json:"request_seq,omitempty"`
	Success         bool            `json:"success,omitempty"`
	Command         string          `json:"command,omitempty"`
	Event           string          `json:"event,omitempty"`
	Arguments       json.RawMessage `json:"arguments,omitempty"`
	Body            json.RawMessage `json:"body,omitempty"`
}

// Stream reads and writes Content-Length framed messages.
type Stream struct {
	reader *bufio.Reader
	writer io.Writer
	mu     sync.Mutex
}

// NewStream wraps a protocol connection.
func NewStream(reader io.Reader, writer io.Writer) *Stream {
	return &Stream{reader: bufio.NewReader(reader), writer: writer}
}

// Read waits for one message.
func (s *Stream) Read() (Message, error) {
	length := -1
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return Message{}, fmt.Errorf("read protocol header: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		name, value, found := strings.Cut(line, ":")
		if !found || !strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			continue
		}
		length, err = strconv.Atoi(strings.TrimSpace(value))
		if err != nil || length < 0 {
			return Message{}, fmt.Errorf("read protocol header: invalid content length %q", value)
		}
	}
	if length < 0 {
		return Message{}, fmt.Errorf("read protocol header: missing content length")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(s.reader, payload); err != nil {
		return Message{}, fmt.Errorf("read protocol message: %w", err)
	}
	var message Message
	if err := json.Unmarshal(payload, &message); err != nil {
		return Message{}, fmt.Errorf("decode protocol message: %w", err)
	}
	return message, nil
}

// Write sends a message.
func (s *Stream) Write(message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode protocol message: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return fmt.Errorf("write protocol header: %w", err)
	}
	if _, err := s.writer.Write(payload); err != nil {
		return fmt.Errorf("write protocol message: %w", err)
	}
	return nil
}
