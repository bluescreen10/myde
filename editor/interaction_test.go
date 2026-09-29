package editor

import (
	"bytes"
	"errors"
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
	"github.com/bluescreen10/myde/ui"
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
	panel := newSidebarPanel(ui.Sidebar{
		Title: "Changes",
		Sections: []ui.SidebarSection{{Title: "Files", Items: []ui.SidebarItem{
			{Label: "a", Value: "a", Kind: "unstaged"},
			{Label: "b", Value: "b", Kind: "unstaged"},
		}}},
		SelectedValue:   "b",
		SelectedKind:    "unstaged",
		RefreshInterval: time.Millisecond,
		OnRefresh: func() (ui.Sidebar, error) {
			refreshes++
			return ui.Sidebar{
				Title: "Changes",
				Sections: []ui.SidebarSection{{Title: "Files", Items: []ui.SidebarItem{
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

func TestLSPCompletionKeepsArrowNavigationAndRejectsOldResponses(t *testing.T) {
	current := buffer.New()
	app := &App{
		buffers:         []*editorBuffer{{text: current}},
		completionEpoch: 2,
		palette: &palette{
			completion:        true,
			completionPending: true,
			items:             []paletteItem{{label: "old"}},
			filtered:          []paletteItem{{label: "old"}},
		},
	}
	point := current.Cursors()[0].Point
	app.handleServerEvent(serverEvent{
		completionReady:    true,
		completionBuffer:   current,
		completionRevision: current.Revision(),
		completionPoint:    point,
		completionEpoch:    1,
		completions:        []lspCompletion{{label: "stale"}},
	})
	if app.palette == nil || app.palette.filtered[0].label != "old" {
		t.Fatalf("stale completion response replaced the pending menu: %#v", app.palette)
	}

	app.handleServerEvent(serverEvent{
		completionReady:    true,
		completionBuffer:   current,
		completionRevision: current.Revision(),
		completionPoint:    point,
		completionEpoch:    2,
		completions: []lspCompletion{
			{label: "first"},
			{label: "second"},
		},
	})
	if app.palette == nil || app.palette.completionPending {
		t.Fatalf("current completion response did not activate the menu: %#v", app.palette)
	}
	before := current.Cursors()[0].Point
	if err := app.handleEvent(terminal.Event{Key: terminal.KeyDown}); err != nil {
		t.Fatal(err)
	}
	if app.palette == nil {
		t.Fatal("down arrow closed the completion menu")
	}
	if app.palette.selected != 1 {
		t.Fatalf("down arrow selected index %d, want 1", app.palette.selected)
	}
	if after := current.Cursors()[0].Point; after != before {
		t.Fatalf("down arrow moved editor cursor from %+v to %+v", before, after)
	}
	if err := app.handleEvent(terminal.Event{Key: terminal.KeyIgnored}); err != nil {
		t.Fatal(err)
	}
	if app.palette == nil || app.palette.selected != 1 {
		t.Fatalf("key release closed the completion menu: %#v", app.palette)
	}
}

func TestEmptyLSPCompletionClosesPendingMenu(t *testing.T) {
	current := buffer.New()
	app := &App{
		buffers:         []*editorBuffer{{text: current}},
		completionEpoch: 3,
		palette: &palette{
			completion:        true,
			completionPending: true,
			items:             []paletteItem{{label: "old"}},
			filtered:          []paletteItem{{label: "old"}},
		},
	}
	app.handleServerEvent(serverEvent{
		completionReady:    true,
		completionBuffer:   current,
		completionRevision: current.Revision(),
		completionPoint:    current.Cursors()[0].Point,
		completionEpoch:    3,
	})
	if app.palette != nil {
		t.Fatalf("empty completion response left menu open: %#v", app.palette)
	}
}

func TestCompletionRefreshEvents(t *testing.T) {
	for _, event := range []terminal.Event{
		{Key: terminal.KeyRune, Rune: 'a'},
		{Key: terminal.KeyRune, Rune: '.'},
		{Key: terminal.KeyBackspace},
		{Key: terminal.KeyDelete},
	} {
		if !shouldRefreshLSPCompletion(event) {
			t.Errorf("event %+v did not refresh completion", event)
		}
	}
	for _, event := range []terminal.Event{
		{Key: terminal.KeyRune, Rune: ' '},
		{Key: terminal.KeyLeft},
		{Key: terminal.KeyRune, Rune: 'a', Control: true},
	} {
		if shouldRefreshLSPCompletion(event) {
			t.Errorf("event %+v unexpectedly refreshed completion", event)
		}
	}
}

func TestTypingSuppressesDiagnosticCardUntilCursorMoves(t *testing.T) {
	current := buffer.New()
	current.Insert(0, []byte("bad"))
	point := buffer.Point{Column: 2}
	current.SetCursors([]buffer.Cursor{{Anchor: point, Point: point}})
	app := &App{
		buffers: []*editorBuffer{{
			text:        current,
			highlighter: syntax.NewLanguage("plain"),
			diagnostics: []diagnostic{{line: 0, column: 0, endLine: 0, endColumn: 10, severity: 1}},
		}},
		screen:         terminal.NewScreen(io.Discard, 80, 24),
		showDiagnostic: true,
	}
	if _, ok := app.visibleDiagnosticAtCursor(); !ok {
		t.Fatal("diagnostic card was not initially eligible")
	}
	if err := app.handleEvent(terminal.Event{Key: terminal.KeyRune, Rune: 'x'}); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.visibleDiagnosticAtCursor(); ok {
		t.Fatal("typing allowed a diagnostic card to replace completion UI")
	}
	if err := app.handleEvent(terminal.Event{Key: terminal.KeyLeft}); err != nil {
		t.Fatal(err)
	}
	if _, ok := app.visibleDiagnosticAtCursor(); !ok {
		t.Fatal("explicit cursor movement did not re-enable the diagnostic card")
	}

	app.chooseCompletion("LSP Completion", []paletteItem{{label: "Print"}}, false, func(paletteItem) {})
	if _, ok := app.visibleDiagnosticAtCursor(); ok {
		t.Fatal("diagnostic card was visible over an active completion menu")
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

func TestPluginTextEditorSubmitsMultilineContentOnSave(t *testing.T) {
	plain := modeKey("Plain Text")
	base := buffer.NewNamed("scratch")
	app := &App{
		buffers: []*editorBuffer{{
			text: base, highlighter: syntax.NewLanguage("plain"), mode: plain,
		}},
		modes: map[string]plugin.Mode{
			plain: {Name: "Plain Text", Syntax: "plain"},
		},
		extensionModes: make(map[string]string),
	}
	fail := true
	var submitted []byte
	app.OpenTextEditor("COMMIT_EDITMSG", nil, func(content []byte) error {
		if fail {
			return errors.New("commit failed")
		}
		submitted = append([]byte(nil), content...)
		app.message = "commit created"
		return nil
	})
	if got := app.current().Name(); got != "COMMIT_EDITMSG" {
		t.Fatalf("active buffer = %q, want COMMIT_EDITMSG", got)
	}
	message := []byte("subject\n\nbody line one\nbody line two\n")
	app.current().Insert(0, message)
	if err := app.save(""); err == nil || err.Error() != "commit failed" {
		t.Fatalf("failed submission error = %v", err)
	}
	if app.current().Name() != "COMMIT_EDITMSG" {
		t.Fatal("failed submission closed the text editor")
	}

	fail = false
	if err := app.save(""); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(submitted, message) {
		t.Fatalf("submitted content = %q, want %q", submitted, message)
	}
	if len(app.buffers) != 1 || app.current() != base {
		t.Fatalf("successful submission left buffers = %#v", app.buffers)
	}
	if app.message != "commit created" {
		t.Fatalf("submission message = %q, want commit created", app.message)
	}
}
