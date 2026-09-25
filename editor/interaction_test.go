package editor

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/syntax"
	"github.com/bluescreen10/myde/terminal"
)

func TestDefaultPageBindingsAndCommands(t *testing.T) {
	bindings := defaultBindings()
	if bindings["ctrl-shift-f"] != "search.project" {
		t.Fatalf("global search binding = %q", bindings["ctrl-shift-f"])
	}
	if bindings["alt-shift-up"] != "cursor.page-up" ||
		bindings["alt-shift-down"] != "cursor.page-down" {
		t.Fatalf("page bindings = %q, %q", bindings["alt-shift-up"], bindings["alt-shift-down"])
	}
	current := buffer.New()
	current.Insert(0, []byte(strings.Repeat("line\n", 30)))
	point := buffer.Point{Line: 20}
	current.SetCursors([]buffer.Cursor{{Anchor: point, Point: point}})
	app := &App{
		buffers: []*editorBuffer{{text: current}},
		screen:  terminal.NewScreen(io.Discard, 80, 10),
	}
	if err := app.pageUp(""); err != nil {
		t.Fatal(err)
	}
	if got := current.Cursors()[0].Point.Line; got != 13 {
		t.Fatalf("page up line = %d, want 13", got)
	}
	if err := app.pageDown(""); err != nil {
		t.Fatal(err)
	}
	if got := current.Cursors()[0].Point.Line; got != 20 {
		t.Fatalf("page down line = %d, want 20", got)
	}
}

func TestCommandPaletteHasNoMarkerAndOrdersRecentItemsFirst(t *testing.T) {
	app := &App{
		files:          []string{"a.go", "b.go", "c.go"},
		recentFiles:    []string{"c.go", "a.go"},
		recentCommands: []string{"second", "first"},
		commands: map[string]plugin.Command{
			"first":  func(string) error { return nil },
			"second": func(string) error { return nil },
			"third":  func(string) error { return nil },
		},
		extensions: &extensions{functions: make(map[string][]string)},
	}
	if err := app.commandPalette(""); err != nil {
		t.Fatal(err)
	}
	if !app.palette.hideQueryMarker {
		t.Fatal("command palette query marker is visible")
	}
	files, _ := app.palette.source("")
	if files[0].value != "c.go" || files[1].value != "a.go" || files[2].value != "b.go" {
		t.Fatalf("file order = %q, %q, %q", files[0].value, files[1].value, files[2].value)
	}
	commands, query := app.palette.source(">")
	if query != "" || commands[0].value != "second" || commands[1].value != "first" || commands[2].value != "third" {
		t.Fatalf("command order = %q, %q, %q; query = %q",
			commands[0].value, commands[1].value, commands[2].value, query)
	}
}

func TestAddNextMatchAndEscapeCursorLayers(t *testing.T) {
	current := buffer.New()
	current.Insert(0, []byte("word x word"))
	current.SetCursors([]buffer.Cursor{{
		Anchor: current.Point(0), Point: current.Point(4),
	}})
	app := &App{
		buffers: []*editorBuffer{{text: current}},
		screen:  terminal.NewScreen(io.Discard, 80, 24),
	}
	if err := app.addCursorAtNextMatch(""); err != nil {
		t.Fatal(err)
	}
	cursors := current.Cursors()
	if len(cursors) != 2 || current.Offset(cursors[1].Anchor) != 7 || current.Offset(cursors[1].Point) != 11 {
		t.Fatalf("cursors after add = %+v", cursors)
	}
	if err := app.addCursorAtNextMatch(""); err != nil {
		t.Fatal(err)
	}
	if cursors = current.Cursors(); len(cursors) != 2 {
		t.Fatalf("duplicate cursor was added: %+v", cursors)
	}
	app.cancelCursorState()
	cursors = current.Cursors()
	if len(cursors) != 1 || cursors[0].Anchor == cursors[0].Point {
		t.Fatalf("first escape cursors = %+v", cursors)
	}
	app.cancelCursorState()
	cursors = current.Cursors()
	if len(cursors) != 1 || cursors[0].Anchor != cursors[0].Point {
		t.Fatalf("second escape cursors = %+v", cursors)
	}
}

func TestMultipleCursorsUseSoftwareCursorPositions(t *testing.T) {
	current := buffer.New()
	current.Insert(0, []byte("abc\ndef"))
	current.SetCursors([]buffer.Cursor{
		{Point: buffer.Point{Line: 0, Column: 1}},
		{Point: buffer.Point{Line: 1, Column: 2}},
	})
	app := &App{
		buffers: []*editorBuffer{{text: current}},
		screen:  terminal.NewScreen(io.Discard, 80, 24),
		browser: newFileBrowser("", nil, nil),
	}
	if !app.usesSoftwareCursors() {
		t.Fatal("multiple editor cursors did not select software rendering")
	}
	firstX, firstY := app.bufferPointPosition(current.Cursors()[0].Point, 0)
	secondX, secondY := app.bufferPointPosition(current.Cursors()[1].Point, 0)
	if firstX != 4 || firstY != 1 || secondX != 5 || secondY != 2 {
		t.Fatalf("cursor positions = (%d,%d), (%d,%d)", firstX, firstY, secondX, secondY)
	}

	app.minibuffer = &minibuffer{}
	if app.usesSoftwareCursors() {
		t.Fatal("software cursors remained active while the minibuffer had focus")
	}
}

func TestSidebarRefreshPreservesSelection(t *testing.T) {
	refreshes := 0
	panel := newSidebarPanel(plugin.Sidebar{
		Title: "Changes",
		Sections: []plugin.SidebarSection{{Title: "Files", Items: []plugin.SidebarItem{
			{Label: "a", Value: "a", Kind: "unstaged"},
			{Label: "b", Value: "b", Kind: "unstaged"},
		}}},
		SelectedValue:   "b",
		SelectedKind:    "unstaged",
		RefreshInterval: time.Millisecond,
		OnRefresh: func() (plugin.Sidebar, error) {
			refreshes++
			return plugin.Sidebar{
				Title: "Changes",
				Sections: []plugin.SidebarSection{{Title: "Files", Items: []plugin.SidebarItem{
					{Label: "b", Value: "b", Kind: "unstaged"},
					{Label: "c", Value: "c", Kind: "unstaged"},
				}}},
				RefreshInterval: time.Millisecond,
			}, nil
		},
	})
	panel.nextRefresh = time.Time{}
	app := &App{sidebar: panel, servers: make(chan serverEvent, 1)}
	app.pollSidebarRefresh()
	select {
	case event := <-app.servers:
		app.handleServerEvent(event)
	case <-time.After(time.Second):
		t.Fatal("sidebar refresh did not complete")
	}
	selected, ok := app.sidebar.selectedItem()
	if refreshes != 1 || !ok || selected.Value != "b" || selected.Kind != "unstaged" {
		t.Fatalf("refreshes = %d, selected = %+v, ok = %v", refreshes, selected, ok)
	}
}

func TestSavePreservesScrollState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	current, err := buffer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	current.Insert(current.Len(), []byte("after\n"))
	plain := modeKey("Plain Text")
	app := &App{
		root: root,
		buffers: []*editorBuffer{{
			text: current, highlighter: syntax.NewLanguage("plain"), mode: plain,
		}},
		modes:      map[string]plugin.Mode{plain: {Name: "Plain Text", Syntax: "plain"}},
		extensions: &extensions{},
		topLine:    17,
		leftColumn: 9,
	}
	if err := app.save(""); err != nil {
		t.Fatal(err)
	}
	if app.topLine != 17 || app.leftColumn != 9 {
		t.Fatalf("scroll after save = (%d, %d), want (17, 9)", app.topLine, app.leftColumn)
	}
}
