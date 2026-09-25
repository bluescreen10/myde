package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DebugProcess manages a Debug Adapter Protocol subprocess.
type DebugProcess struct {
	command *exec.Cmd
	stream  *Stream
	closer  io.Closer

	nextSequence atomic.Int64
	pendingMu    sync.Mutex
	pending      map[int64]chan Message
	events       chan Notification
}

// StartDebugReverse launches a debug adapter that connects back to a local TCP listener.
// Every {address} token in the arguments is replaced with the listener address.
func StartDebugReverse(ctx context.Context, command string, arguments ...string) (*DebugProcess, error) {
	arguments = append([]string(nil), arguments...)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for debug adapter: %w", err)
	}
	defer listener.Close()
	address := listener.Addr().String()
	for index := range arguments {
		arguments[index] = strings.ReplaceAll(arguments[index], "{address}", address)
	}

	cmd := exec.CommandContext(ctx, command, arguments...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start debug adapter: %w", err)
	}
	if tcpListener, ok := listener.(*net.TCPListener); ok {
		_ = tcpListener.SetDeadline(time.Now().Add(5 * time.Second))
	}
	connection, err := listener.Accept()
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("accept debug adapter connection: %w", err)
	}
	return newDebugProcess(cmd, connection, connection), nil
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
	return newDebugProcess(cmd, output, input), nil
}

func newDebugProcess(command *exec.Cmd, reader io.Reader, writer io.Writer) *DebugProcess {
	process := &DebugProcess{
		command: command,
		stream:  NewStream(reader, writer),
		pending: make(map[int64]chan Message),
		events:  make(chan Notification, 64),
	}
	if closer, ok := reader.(io.Closer); ok {
		process.closer = closer
	}
	go process.readMessages()
	go func() {
		_ = command.Wait()
	}()
	return process
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
	if p.closer != nil {
		_ = p.closer.Close()
	}
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
