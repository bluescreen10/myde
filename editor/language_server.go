package editor

import (
	"context"
	"fmt"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/protocol"
)

// A session belongs to one editing mode and is shared by its open documents.
type languageServerSession struct {
	mode              *editingMode
	process           *protocol.Process
	cancel            context.CancelFunc
	syncKind          int
	completionResolve bool
	messages          []string
	lastError         string
}

type languageServerMessage struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}

func (s *languageServerSession) recordMessage(message string) {
	s.messages = append(s.messages, message)
	if len(s.messages) > 200 {
		s.messages = s.messages[len(s.messages)-200:]
	}
}

func (a *App) languageServerForBuffer(current *buffer.Buffer) *languageServerSession {
	editorBuffer := a.editorBufferFor(current)
	if editorBuffer == nil {
		return nil
	}
	mode := a.modes[editorBuffer.mode]
	if mode == nil || mode.languageServer == nil || !mode.languageServer.process.Running() {
		return nil
	}
	return mode.languageServer
}

func (a *App) registeredMode(name string) (*editingMode, error) {
	mode := a.modes[modeKey(name)]
	if mode == nil {
		return nil, fmt.Errorf("unknown mode %q", name)
	}
	return mode, nil
}

func (m *editingMode) languageServerProgram() plugin.Program {
	if m.serverProgram.Command != "" {
		return m.serverProgram
	}
	return m.LanguageServer
}

// StartLanguageServer starts a mode's configured server, reusing it if running.
func (a *App) StartLanguageServer(name string) error {
	mode, err := a.registeredMode(name)
	if err != nil {
		return err
	}
	if mode.languageServer != nil && mode.languageServer.process.Running() {
		return nil
	}
	program := mode.languageServerProgram()
	if program.Command == "" {
		return fmt.Errorf("mode %s has no language server", mode.Name)
	}
	return a.startLanguageServer(program, mode)
}

// RunLanguageServer replaces only this mode's server with the supplied program.
func (a *App) RunLanguageServer(name string, program plugin.Program) error {
	mode, err := a.registeredMode(name)
	if err != nil {
		return err
	}
	if program.Command == "" {
		return fmt.Errorf("language server command cannot be empty")
	}
	return a.startLanguageServer(program, mode)
}

// StopLanguageServer stops one mode and suppresses automatic starts until an
// explicit Start, Run, or Restart call. Other modes keep their sessions.
func (a *App) StopLanguageServer(name string) error {
	mode, err := a.registeredMode(name)
	if err != nil {
		return err
	}
	mode.serverStopped = true
	a.clearLanguageServer(mode)
	a.message = mode.Name + " language server stopped"
	return nil
}

// RestartLanguageServer reopens this mode's documents in a fresh server.
func (a *App) RestartLanguageServer(name string) error {
	mode, err := a.registeredMode(name)
	if err != nil {
		return err
	}
	program := mode.languageServerProgram()
	if program.Command == "" {
		return fmt.Errorf("mode %s has no language server", mode.Name)
	}
	return a.startLanguageServer(program, mode)
}

func (a *App) clearLanguageServer(mode *editingMode) {
	session := mode.languageServer
	mode.languageServer = nil
	if session == nil {
		return
	}
	session.cancel()
	for _, current := range a.buffers {
		if current.lsp == session {
			current.lsp = nil
			current.lspURI = ""
			current.diagnostics = nil
		}
	}
	if len(a.buffers) > 0 && a.currentEditorBuffer().mode == modeKey(mode.Name) {
		a.completionEpoch++
		if a.palette != nil && a.palette.completion {
			a.palette = nil
		}
	}
}
