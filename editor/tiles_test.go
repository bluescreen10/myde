package editor

import (
	"io"
	"testing"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/terminal"
)

func newTilingTestApp(names ...string) *App {
	app := &App{
		screen: terminal.NewScreen(io.Discard, 80, 24),
		modes:  make(map[string]plugin.Mode),
	}
	for _, name := range names {
		app.buffers = append(app.buffers, &editorBuffer{text: buffer.NewNamed(name)})
	}
	return app
}

func TestSplitCommandsCreateExpectedLayouts(t *testing.T) {
	app := newTilingTestApp("one")
	app.registerCommands()
	for _, name := range []string{
		"view.split-horizontally", "view.split-vertically", "view.next", "view.previous", "view.close",
	} {
		if app.commands[name] == nil {
			t.Fatalf("command %q was not registered", name)
		}
	}

	if err := app.execute("view.split-horizontally"); err != nil {
		t.Fatal(err)
	}
	if app.tileRoot == nil || app.tileRoot.direction != splitHorizontally || app.focusedTile != app.tileRoot.second {
		t.Fatalf("horizontal split = %+v, focus = %p", app.tileRoot, app.focusedTile)
	}
	first, second, separator, vertical := splitEditorRegion(
		editorRegion{x: 0, y: 1, width: 80, height: 22}, app.tileRoot.direction,
	)
	if vertical || first.height != 10 || separator.y != 11 || second.height != 11 {
		t.Fatalf("horizontal regions = first %+v, separator %+v, second %+v", first, separator, second)
	}

	if err := app.closeView(""); err != nil {
		t.Fatal(err)
	}
	if app.tileRoot != nil || app.focusedTile != nil {
		t.Fatal("closing one of two panes did not restore the unsplit editor")
	}
	if err := app.splitVertically(""); err != nil {
		t.Fatal(err)
	}
	first, second, separator, vertical = splitEditorRegion(
		editorRegion{x: 0, y: 1, width: 80, height: 22}, app.tileRoot.direction,
	)
	if !vertical || first.width != 39 || separator.x != 39 || second.width != 40 {
		t.Fatalf("vertical regions = first %+v, separator %+v, second %+v", first, separator, second)
	}
}

func TestSplitFocusKeepsIndependentBufferAndViewport(t *testing.T) {
	app := newTilingTestApp("one", "two")
	app.screen = nil // Keep the test focused on state swapping, not cursor clamping.
	app.topLine = 3
	app.leftColumn = 2
	if err := app.splitVertically(""); err != nil {
		t.Fatal(err)
	}
	app.topLine = 17
	app.leftColumn = 9
	app.activateBufferIndex(1, false)

	if err := app.previousView(""); err != nil {
		t.Fatal(err)
	}
	if app.active != 0 || app.topLine != 3 || app.leftColumn != 2 {
		t.Fatalf("first pane state = buffer %d, viewport (%d, %d)", app.active, app.topLine, app.leftColumn)
	}
	if err := app.nextView(""); err != nil {
		t.Fatal(err)
	}
	if app.active != 1 || app.topLine != 17 || app.leftColumn != 9 {
		t.Fatalf("second pane state = buffer %d, viewport (%d, %d)", app.active, app.topLine, app.leftColumn)
	}
}

func TestClosingSplitBufferCollapsesItsPane(t *testing.T) {
	app := newTilingTestApp("one", "two")
	if err := app.splitVertically(""); err != nil {
		t.Fatal(err)
	}
	app.activateBufferIndex(1, true)
	app.topLine = 12
	if err := app.previousView(""); err != nil {
		t.Fatal(err)
	}
	if !app.removeBuffer(app.current()) {
		t.Fatal("buffer was not removed")
	}
	if len(app.buffers) != 1 || app.current().Name() != "two" {
		t.Fatalf("remaining buffers = %d, current = %q", len(app.buffers), app.current().Name())
	}
	if app.tileRoot != nil || app.focusedTile != nil {
		t.Fatal("a lone split pane was not collapsed")
	}
	if app.topLine != 12 {
		t.Fatalf("surviving viewport top = %d, want 12", app.topLine)
	}
}

func TestNestedViewCloseOnlyCollapsesFocusedBranch(t *testing.T) {
	app := newTilingTestApp("one")
	if err := app.splitVertically(""); err != nil {
		t.Fatal(err)
	}
	if err := app.splitHorizontally(""); err != nil {
		t.Fatal(err)
	}
	if err := app.closeView(""); err != nil {
		t.Fatal(err)
	}
	leaves := make([]*editorTile, 0, 2)
	app.tileRoot.leaves(&leaves)
	if len(leaves) != 2 || app.tileRoot.direction != splitVertically {
		t.Fatalf("nested close left %d panes with root direction %v", len(leaves), app.tileRoot.direction)
	}
}

func TestTerminalSizeUsesItsSplitRegion(t *testing.T) {
	app := newTilingTestApp("text", "terminal")
	shell := &shellBuffer{}
	app.buffers[1].terminal = shell
	if err := app.splitVertically(""); err != nil {
		t.Fatal(err)
	}
	app.activateBufferIndex(1, true)
	width, height := app.terminalSizeForShell(shell)
	if width != 40 || height != 22 {
		t.Fatalf("terminal split size = %dx%d, want 40x22", width, height)
	}
}
