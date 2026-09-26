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
	NewView(view View) ViewHandle
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

// Sidebar describes an ephemeral, keyboard-driven panel on the left.
type Sidebar struct {
	Title           string
	Sections        []SidebarSection
	SelectedValue   string
	SelectedKind    string
	Help            []KeyHelp
	OnAction        func(action Action, item SidebarItem) error
	RefreshInterval time.Duration
	OnRefresh       func() (Sidebar, error)
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

// View is a persistent tab composed from UI panes. OnRefresh runs in the
// background and should return self-contained data rather than call Host.
type View struct {
	Title           string
	Layout          Layout
	RefreshInterval time.Duration
	OnRefresh       func() (View, error)
}

// ViewHandle controls a tab created by NewView. Show raises an existing view
// and reports false after the tab has been destroyed.
type ViewHandle interface {
	Show() bool
	Update(view View) bool
	Destroy()
}

// Layout arranges panes along one axis. Weight controls their relative size.
type Layout struct {
	Direction LayoutDirection
	Panes     []Pane
}

// LayoutDirection identifies the axis used to arrange a layout's panes.
type LayoutDirection string

const (
	LayoutHorizontal LayoutDirection = "horizontal"
	LayoutVertical   LayoutDirection = "vertical"
)

// Pane is a bordered region containing one widget.
type Pane struct {
	ID       string
	Title    string
	Weight   int
	List     *List
	Document *ViewDocument
}

// List is a selectable list widget. OnSelect is evaluated in the background
// and places its returned document in PreviewPane.
type List struct {
	Sections      []ListSection
	SelectedValue string
	SelectedKind  string
	Help          []KeyHelp
	PreviewPane   string
	OnAction      func(action Action, item ListItem) error
	OnSelect      func(item ListItem) (ViewDocument, error)
}

// ListSection groups list widget items under a heading.
type ListSection struct {
	Title string
	Items []ListItem
}

// ListItem is one selectable row in a list widget.
type ListItem struct {
	Label      string
	Detail     string
	DetailTone Tone
	Value      string
	Kind       string
	Data       string
}

// ViewDocument is immutable content rendered by a document widget.
type ViewDocument struct {
	Title   string
	Content []byte
	Syntax  string
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
