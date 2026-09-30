package editor

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/ui"
)

// Root returns the absolute workspace root.
func (a *App) Root() string {
	return a.root
}

// CurrentPath returns the event buffer's backing path while dispatching an
// event, or the active buffer's path otherwise.
func (a *App) CurrentPath() string {
	return a.pluginEditorBuffer().text.Path()
}

// CurrentDocument returns an immutable snapshot of the event buffer while
// dispatching an event, or the active buffer otherwise.
func (a *App) CurrentDocument() plugin.Document {
	current := a.pluginEditorBuffer().text
	return plugin.Document{
		Path:     current.Path(),
		Content:  current.Bytes(),
		Dirty:    current.IsDirty(),
		ReadOnly: current.IsReadOnly(),
	}
}

// ReplaceCurrentDocument replaces the event buffer while dispatching an event,
// or the active buffer otherwise, as one undoable edit.
func (a *App) ReplaceCurrentDocument(content []byte) error {
	editorBuffer := a.pluginEditorBuffer()
	current := editorBuffer.text
	if current.IsReadOnly() {
		return fmt.Errorf("%s is read-only", current.Name())
	}
	if editorBuffer.terminal != nil {
		return fmt.Errorf("terminal buffers cannot be replaced by plugins")
	}
	before := current.Bytes()
	if bytes.Equal(before, content) {
		return nil
	}
	a.showDiagnostic = false
	offsets := make([]int, len(current.Cursors()))
	for index, cursor := range current.Cursors() {
		offsets[index] = current.Offset(cursor.Point)
	}
	current.BeginTransaction()
	current.Delete(0, current.Len())
	current.Insert(0, content)
	current.EndTransaction()
	cursors := make([]buffer.Cursor, len(offsets))
	for index, offset := range offsets {
		point := current.Point(min(offset, current.Len()))
		cursors[index] = buffer.Cursor{Anchor: point, Point: point}
	}
	current.SetCursors(cursors)
	editorBuffer.highlighter = a.highlighterForBuffer(current)
	a.notifyLSPFullChange(current)
	if editorBuffer == a.currentEditorBuffer() {
		a.ensureCursorVisible()
	}
	return nil
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
func (a *App) OpenSidebar(sidebar ui.Sidebar) {
	a.closeFileBrowser()
	a.closeWorkspaceSearch()
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
	a.closeWorkspaceSearch()
	view := buffer.NewReadOnly(name, content)
	for index, editorBuffer := range a.buffers {
		current := editorBuffer.text
		if !current.IsReadOnly() || current.Name() != name {
			continue
		}
		previous := a.buffers[index]
		a.buffers[index] = a.newEditorBuffer(view)
		a.replaceEditorBufferInTiles(previous, a.buffers[index])
		a.activateBufferIndex(index, true)
		return
	}
	a.addBuffer(view)
	a.topLine = 0
	a.leftColumn = 0
}

// OpenTextEditor opens or focuses an editable, memory-backed buffer. Saving
// submits its content and closes the buffer when the callback succeeds.
func (a *App) OpenTextEditor(name string, content []byte, submit func([]byte) error) {
	name = strings.TrimSpace(name)
	if name == "" || submit == nil {
		return
	}
	a.CloseSidebar()
	a.closeWorkspaceSearch()
	for index, editorBuffer := range a.buffers {
		if editorBuffer.onSave == nil || editorBuffer.text.Name() != name {
			continue
		}
		a.activateBufferIndex(index, true)
		return
	}
	view := buffer.NewNamed(name)
	if len(content) > 0 {
		view.Insert(0, content)
	}
	a.addBuffer(view)
	a.currentEditorBuffer().onSave = submit
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
