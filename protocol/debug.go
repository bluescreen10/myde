package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"
)

// DebugProcess manages a Debug Adapter Protocol subprocess.
type DebugProcess struct {
	command *exec.Cmd
	stream  *Stream

	nextSequence atomic.Int64
	pendingMu    sync.Mutex
	pending      map[int64]chan Message
	events       chan Notification
}

// StartDebug launches a debug adapter over standard I/O.
func StartDebug(ctx context.Context, command string, arguments ...string) (*DebugProcess, error) {
	cmd := exec.CommandContext(ctx, command, arguments...)
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open debug adapter input: %w", err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open debug adapter output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start debug adapter: %w", err)
	}
	process := &DebugProcess{
		command: cmd,
		stream:  NewStream(output, input),
		pending: make(map[int64]chan Message),
		events:  make(chan Notification, 64),
	}
	go process.readMessages()
	go func() {
		_ = cmd.Wait()
	}()
	return process, nil
}

// Request sends a DAP request and waits for its response.
func (p *DebugProcess) Request(ctx context.Context, command string, arguments any, body any) error {
	sequence := p.nextSequence.Add(1)
	response := make(chan Message, 1)
	p.pendingMu.Lock()
	p.pending[sequence] = response
	p.pendingMu.Unlock()

	if err := p.stream.Write(map[string]any{
		"seq":       sequence,
		"type":      "request",
		"command":   command,
		"arguments": arguments,
	}); err != nil {
		p.removePending(sequence)
		return err
	}
	select {
	case <-ctx.Done():
		p.removePending(sequence)
		return ctx.Err()
	case message := <-response:
		if !message.Success {
			return fmt.Errorf("debug adapter rejected %s", command)
		}
		if body != nil && len(message.Body) > 0 {
			if err := json.Unmarshal(message.Body, body); err != nil {
				return fmt.Errorf("decode %s response: %w", command, err)
			}
		}
		return nil
	}
}

func (p *DebugProcess) removePending(sequence int64) {
	p.pendingMu.Lock()
	delete(p.pending, sequence)
	p.pendingMu.Unlock()
}

// Events returns debug adapter events.
func (p *DebugProcess) Events() <-chan Notification {
	return p.events
}

// Close terminates the debug adapter.
func (p *DebugProcess) Close() error {
	if p.command.Process == nil {
		return nil
	}
	if err := p.command.Process.Kill(); err != nil {
		return fmt.Errorf("stop debug adapter: %w", err)
	}
	return nil
}

func (p *DebugProcess) readMessages() {
	defer close(p.events)
	for {
		message, err := p.stream.Read()
		if err != nil {
			return
		}
		if message.Type == "response" {
			p.pendingMu.Lock()
			response := p.pending[int64(message.RequestSequence)]
			delete(p.pending, int64(message.RequestSequence))
			p.pendingMu.Unlock()
			if response != nil {
				response <- message
			}
			continue
		}
		if message.Type == "event" {
			p.events <- Notification{Event: message.Event, Body: message.Body}
		}
	}
}
