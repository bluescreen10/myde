// Package plugin defines the boundary between myde and editor plugins.
package plugin

import "time"

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
	OpenSidebar(sidebar Sidebar)
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

// Sidebar describes an ephemeral, keyboard-driven panel. FullScreen gives the
// panel a second, preview pane and lets Tab move focus between the two panes.
// OnPreview runs in the background and should return self-contained data rather
// than calling UI methods on Host.
type Sidebar struct {
	Title           string
	Sections        []SidebarSection
	SelectedValue   string
	SelectedKind    string
	FullScreen      bool
	Help            []KeyHelp
	OnAction        func(action Action, item SidebarItem) error
	OnPreview       func(item SidebarItem) (SidebarPreview, error)
	RefreshInterval time.Duration
	OnRefresh       func() (Sidebar, error)
}

// SidebarPreview is the document shown beside a full-screen sidebar.
type SidebarPreview struct {
	Title   string
	Content []byte
	Syntax  string
}

// SidebarSection groups related panel items under a heading.
type SidebarSection struct {
	Title string
	Items []SidebarItem
}

// SidebarItem is one selectable row in a sidebar.
type SidebarItem struct {
	Label      string
	Detail     string
	DetailTone Tone
	Value      string
	Kind       string
	Data       string
}

// Tone gives text a semantic theme color.
type Tone string

const (
	ToneDefault Tone = ""
	ToneSuccess Tone = "success"
	ToneWarning Tone = "warning"
	ToneDanger  Tone = "danger"
)

// KeyHelp describes one shortcut shown in a sidebar footer.
type KeyHelp struct {
	Key   string
	Label string
}

// Action identifies an interaction with the selected sidebar item.
type Action string

const (
	// Activate is emitted for Enter.
	Activate Action = "activate"
	// Add is emitted for +.
	Add Action = "add"
	// Remove is emitted for -.
	Remove Action = "remove"
)
