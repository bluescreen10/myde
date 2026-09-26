// Package plugin defines the boundary between myde and editor plugins.
package plugin

import "github.com/bluescreen10/myde/ui"

// Command handles a named editor command and its unparsed arguments.
type Command func(arguments string) error

// Plugin contributes commands and behavior to an editor host.
type Plugin interface {
	Name() string
	Load(host Host) error
}

// Host exposes editor services that plugins may use.
type Host interface {
	Root() string
	CurrentPath() string
	CurrentDocument() Document
	ReplaceCurrentDocument(content []byte) error
	RegisterCommand(name string, command Command) error
	RegisterMode(mode Mode) error
	NewView(view ui.View) ui.ViewHandle
	OpenSidebar(sidebar ui.Sidebar)
	CloseSidebar()
	OpenReadOnlyBuffer(name string, content []byte)
	Prompt(title string, submit func(string) error)
	SetMessage(message string)
}

// Document is an immutable snapshot of the active buffer.
type Document struct {
	Path     string
	Content  []byte
	Dirty    bool
	ReadOnly bool
}

// Program describes an executable contributed by a plugin.
type Program struct {
	Command   string
	Arguments []string
}

// DebugTransport describes how myde communicates with a debug adapter.
type DebugTransport string

const (
	// DebugStandardIO exchanges DAP messages over the adapter's standard streams.
	DebugStandardIO DebugTransport = "stdio"
	// DebugReverseTCP listens locally and lets the adapter connect to myde.
	// The adapter arguments may contain {address}, which is replaced by the listener address.
	DebugReverseTCP DebugTransport = "reverse-tcp"
)

// DebugAdapter describes a mode's Debug Adapter Protocol executable.
type DebugAdapter struct {
	Program   Program
	Transport DebugTransport
}

// Mode associates file extensions with syntax and language tooling.
type Mode struct {
	Name           string
	Extensions     []string
	Syntax         string
	LanguageID     string
	LanguageServer Program
	DebugAdapter   DebugAdapter
}
