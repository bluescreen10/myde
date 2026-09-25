package editor

import (
	"fmt"
	"strings"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/syntax"
)

// Root returns the absolute workspace root.
func (a *App) Root() string {
	return a.root
}

// CurrentPath returns the active buffer's backing path, if it has one.
func (a *App) CurrentPath() string {
	return a.current().Path()
}

// RegisterCommand adds a plugin command to the command palette.
func (a *App) RegisterCommand(name string, command plugin.Command) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("invalid command name %q", name)
	}
	if command == nil {
		return fmt.Errorf("command %s has no handler", name)
	}
	if _, exists := a.commands[name]; exists {
		return fmt.Errorf("command %s is already registered", name)
	}
	a.commands[name] = command
	return nil
}

// OpenSidebar displays an ephemeral plugin panel on the left.
func (a *App) OpenSidebar(sidebar plugin.Sidebar) {
	a.closeFileBrowser()
	a.sidebar = newSidebarPanel(sidebar)
	a.message = ""
}

// CloseSidebar closes the active plugin panel.
func (a *App) CloseSidebar() {
	a.sidebar = nil
}

// OpenReadOnlyBuffer opens or refreshes a named, memory-backed buffer.
func (a *App) OpenReadOnlyBuffer(name string, content []byte) {
	a.CloseSidebar()
	view := buffer.NewReadOnly(name, content)
	for index, current := range a.buffers {
		if !current.IsReadOnly() || current.Name() != name {
			continue
		}
		delete(a.highlights, current)
		a.buffers[index] = view
		a.highlights[view] = syntax.New(name)
		a.active = index
		a.topLine = 0
		a.leftColumn = 0
		return
	}
	a.addBuffer(view)
	a.topLine = 0
	a.leftColumn = 0
}

// Prompt opens a centered text prompt and reports callback failures in the status line.
func (a *App) Prompt(title string, submit func(string) error) {
	a.prompt(title, func(value string) {
		if err := submit(value); err != nil {
			a.message = err.Error()
		}
	})
}

// SetMessage replaces the status-line message.
func (a *App) SetMessage(message string) {
	a.message = message
}
