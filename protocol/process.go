package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"
)

// Notification is an unsolicited server message.
type Notification struct {
	Method string
	Params json.RawMessage
	Event  string
	Body   json.RawMessage
}

// Process manages a JSON-RPC subprocess, such as a language server.
type Process struct {
	command *exec.Cmd
	stream  *Stream

	nextID    atomic.Int64
	pendingMu sync.Mutex
	pending   map[int64]chan Message
	events    chan Notification
	done      chan struct{}
	handler   func(string, json.RawMessage) (any, error)
}

// ProcessOptions configures the subprocess and handles server-to-client requests.
type ProcessOptions struct {
	Directory string
	OnRequest func(string, json.RawMessage) (any, error)
}

// Start launches command with arguments.
func Start(ctx context.Context, command string, arguments ...string) (*Process, error) {
	return StartWithOptions(ctx, command, ProcessOptions{}, arguments...)
}

// StartWithOptions launches a JSON-RPC process with workspace context.
func StartWithOptions(ctx context.Context, command string, options ProcessOptions, arguments ...string) (*Process, error) {
	cmd := exec.CommandContext(ctx, command, arguments...)
	cmd.Dir = options.Directory
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open %s input: %w", command, err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open %s output: %w", command, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}
	process := &Process{
		command: cmd,
		stream:  NewStream(output, input),
		pending: make(map[int64]chan Message),
		events:  make(chan Notification, 64),
		done:    make(chan struct{}),
		handler: options.OnRequest,
	}
	go process.readMessages()
	go func() {
		_ = cmd.Wait()
	}()
	return process, nil
}

// Request sends a JSON-RPC request and waits for its response.
func (p *Process) Request(ctx context.Context, method string, params any, result any) error {
	id := p.nextID.Add(1)
	response := make(chan Message, 1)
	p.pendingMu.Lock()
	p.pending[id] = response
	p.pendingMu.Unlock()

	if err := p.stream.Write(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}); err != nil {
		p.removePending(id)
		return err
	}
	var message Message
	select {
	case <-ctx.Done():
		p.removePending(id)
		return ctx.Err()
	case <-p.done:
		select {
		case message = <-response:
		default:
			p.removePending(id)
			return fmt.Errorf("%s: language server disconnected", method)
		}
	case message = <-response:
	}
	if len(message.Error) > 0 && string(message.Error) != "null" {
		return fmt.Errorf("%s: %s", method, message.Error)
	}
	if result == nil || len(message.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(message.Result, result); err != nil {
		return fmt.Errorf("decode %s result: %w", method, err)
	}
	return nil
}

// Notify sends a JSON-RPC notification.
func (p *Process) Notify(method string, params any) error {
	return p.stream.Write(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

// Events returns server notifications.
func (p *Process) Events() <-chan Notification {
	return p.events
}

// Running reports whether the protocol connection is still open.
func (p *Process) Running() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Close terminates the subprocess.
func (p *Process) Close() error {
	if p.command.Process == nil {
		return nil
	}
	if err := p.command.Process.Kill(); err != nil {
		return fmt.Errorf("stop protocol process: %w", err)
	}
	return nil
}

func (p *Process) readMessages() {
	defer close(p.done)
	defer close(p.events)
	for {
		message, err := p.stream.Read()
		if err != nil {
			return
		}
		if len(message.ID) > 0 {
			if message.Method != "" {
				response := map[string]any{"jsonrpc": "2.0", "id": message.ID}
				var result any
				var requestErr error
				if p.handler != nil {
					result, requestErr = p.handler(message.Method, message.Params)
				} else {
					requestErr = fmt.Errorf("unsupported client request: %s", message.Method)
				}
				if requestErr != nil {
					response["error"] = map[string]any{"code": -32601, "message": requestErr.Error()}
				} else {
					response["result"] = result
				}
				if p.stream.Write(response) != nil {
					return
				}
				continue
			}
			var id int64
			if json.Unmarshal(message.ID, &id) == nil {
				p.pendingMu.Lock()
				response := p.pending[id]
				delete(p.pending, id)
				p.pendingMu.Unlock()
				if response != nil {
					response <- message
				}
			}
			continue
		}
		p.events <- Notification{
			Method: message.Method,
			Params: message.Params,
			Event:  message.Event,
			Body:   message.Body,
		}
	}
}

func (p *Process) removePending(id int64) {
	p.pendingMu.Lock()
	delete(p.pending, id)
	p.pendingMu.Unlock()
}
