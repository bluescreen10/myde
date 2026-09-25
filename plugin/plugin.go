// Package plugin defines the boundary between myde and editor plugins.
package plugin

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
	RegisterCommand(name string, command Command) error
	OpenSidebar(sidebar Sidebar)
	CloseSidebar()
	OpenReadOnlyBuffer(name string, content []byte)
	Prompt(title string, submit func(string) error)
	SetMessage(message string)
}

// Sidebar describes an ephemeral, keyboard-driven panel on the left.
type Sidebar struct {
	Title         string
	Sections      []SidebarSection
	SelectedValue string
	Help          []KeyHelp
	OnAction      func(action Action, item SidebarItem) error
}

// SidebarSection groups related panel items under a heading.
type SidebarSection struct {
	Title string
	Items []SidebarItem
}

// SidebarItem is one selectable row in a sidebar.
type SidebarItem struct {
	Label  string
	Detail string
	Value  string
	Kind   string
}

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
