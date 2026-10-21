package editor

import (
	"errors"

	"github.com/bluescreen10/myde/terminal"
)

// Execute invokes a registered editor, extension, or plugin command.
// Host calls and event-loop methods must be serialized by the caller.
func (a *App) Execute(command string) error { return a.execute(command) }

// HandleEvent dispatches keyboard or mouse input to the focused editor UI.
func (a *App) HandleEvent(event terminal.Event) error { return a.handleEvent(event) }

// Render paints the current frame using incremental terminal output.
func (a *App) Render() error { return a.render() }

// Update polls workspace changes and applies queued background results.
// Embedded hosts call Update between input events; Run does this automatically.
func (a *App) Update() {
	a.pollChanges()
	for {
		select {
		case event := <-a.servers:
			a.handleServerEvent(event)
		default:
			return
		}
	}
}

// Close releases all plugin processes and terminal shells without saving.
// It is safe to call Close repeatedly. Run calls it when its loop ends.
func (a *App) Close() error {
	a.running = false
	a.CloseSidebar()
	closers := a.pluginClosers
	a.pluginClosers = nil
	var failures []error
	for _, closer := range closers {
		if err := closer.Close(); err != nil {
			failures = append(failures, err)
		}
	}
	for _, mode := range a.modes {
		a.clearLanguageServer(mode)
		if err := a.StopDebugAdapter(mode.Name); err != nil {
			failures = append(failures, err)
		}
	}
	for _, current := range a.buffers {
		if current.terminal != nil && current.terminal.cancel != nil {
			current.terminal.cancel()
		}
	}
	return errors.Join(failures...)
}
