// Package ui defines the editor's shared, theme-independent UI protocol.
package ui

import (
	"strings"
	"time"
)

// View is a persistent tab composed from UI panes. BeforeRefresh captures host
// data on the UI loop; OnRefresh runs in the background and should return
// self-contained data rather than call the host.
type View struct {
	Title  string
	Layout Layout
	// FocusPane optionally chooses focus when a view is shown or updated.
	FocusPane       string
	BeforeRefresh   func()
	RefreshInterval time.Duration
	OnRefresh       func() (View, error)
}

// ViewHandle controls a tab created by a host. Show raises an existing view
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
	ID     string
	Title  string
	Weight int
	List   *List
	Widget Widget
}

// List is a selectable list widget. Input optionally adds an editable query;
// OnInput runs on the UI loop and returns the updated view. OnSelect runs in
// the background and places its returned widget in PreviewPane.
type List struct {
	Input         *TextInput
	OnInput       func(string) (View, error)
	Message       string
	Sections      []ListSection
	SelectedValue string
	SelectedKind  string
	Help          []KeyHelp
	PreviewPane   string
	OnAction      func(action Action, item ListItem) error
	OnSelect      func(item ListItem) (Widget, error)
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

// Widget supplies a theme-independent render model for a pane. Implementations
// own presentation-specific logic while the editor owns terminal painting,
// clipping, scrolling, and theme color resolution.
type Widget interface {
	RenderWidget() WidgetContent
}

// WidgetContent is the immutable result of rendering a widget.
type WidgetContent struct {
	Title string
	Lines []RichTextLine
	// FocusLine and FocusColumn identify the initial preview location, using
	// a zero-based row and display-cell column. Refreshes preserve manual scroll.
	FocusLine   int
	FocusColumn int
}

// RenderWidget lets static rich-text content be used directly as a Widget.
func (content WidgetContent) RenderWidget() WidgetContent {
	return content
}

// RichTextLine is one row of widget output. Its style is also used to fill the
// unused width of the row.
type RichTextLine struct {
	Style WidgetStyle
	Spans []RichTextSpan
}

// RichTextSpan is one styled run. Runs flow after the preceding span unless
// Positioned is true, in which case Column is an absolute display-cell column.
// Widgets never need to emit terminal control sequences.
type RichTextSpan struct {
	Text       string
	Style      WidgetStyle
	Positioned bool
	Column     int
}

// WidgetStyle uses semantic tones so custom widgets remain theme-independent.
// BackgroundIntensity is a percentage blended over the panel background.
type WidgetStyle struct {
	Foreground          Tone
	Background          Tone
	BackgroundIntensity uint8
	Bold                bool
}

// TextWidget is the built-in plain-text widget.
type TextWidget struct {
	Title   string
	Content []byte
}

// RenderWidget renders plain text without presentation-specific styling.
func (widget TextWidget) RenderWidget() WidgetContent {
	text := strings.ReplaceAll(string(widget.Content), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	content := WidgetContent{Title: widget.Title}
	if text == "" {
		return content
	}
	for _, line := range strings.Split(text, "\n") {
		content.Lines = append(content.Lines, RichTextLine{
			Spans: []RichTextSpan{{Text: line}},
		})
	}
	return content
}

// Tone identifies a semantic theme color.
type Tone string

const (
	ToneDefault    Tone = ""
	ToneForeground Tone = "foreground"
	TonePanel      Tone = "panel"
	ToneAccent     Tone = "accent"
	ToneMuted      Tone = "muted"
	ToneSuccess    Tone = "success"
	ToneWarning    Tone = "warning"
	ToneDanger     Tone = "danger"
)

// KeyHelp describes one shortcut shown in a widget footer.
type KeyHelp struct {
	Key   string
	Label string
}

// Action identifies an interaction with a selected list or sidebar item.
type Action string

const (
	// Activate is emitted for Enter.
	Activate Action = "activate"
	// Add is emitted for +.
	Add Action = "add"
	// Remove is emitted for -.
	Remove Action = "remove"
)

// TextInput describes an editable, single-line widget.
type TextInput struct {
	Value       string
	Placeholder string
}

// SidebarState is the state handed back to a sidebar's owner when hidden.
type SidebarState struct {
	Input         string
	SelectedValue string
	SelectedKind  string
	Top           int
}

// Sidebar describes an ephemeral, keyboard-driven panel on the left.
// BeforeRefresh runs on the UI loop to capture host data; OnRefresh runs in
// the background and must use independent snapshots rather than call the host.
type Sidebar struct {
	Message         string
	Width           int
	Input           *TextInput
	OnInput         func(string) (Sidebar, error)
	OnClose         func(SidebarState)
	Top             int
	BeforeRefresh   func()
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
