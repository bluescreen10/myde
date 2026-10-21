package editor_test

import (
	"github.com/bluescreen10/myde/terminal"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitCommandsCreateExpectedLayouts(t *testing.T) {
	h := newHarness(t, "")
	h.replace("visible source")
	h.exec("view.split-horizontally")
	if !strings.Contains(h.screen.row(11), "────") {
		t.Fatalf("horizontal divider=%q", h.screen.row(11))
	}
	h.exec("view.close")
	h.exec("view.split-vertically")
	if !strings.Contains(h.screen.row(5), "│") {
		t.Fatal("vertical divider not rendered")
	}
	h.exec("view.next")
	h.exec("view.previous")
	h.exec("view.close")
	if err := h.app.Execute("view.close"); err == nil {
		t.Fatal("closed unsplit editor")
	}
}
func TestSplitFocusKeepsIndependentBuffers(t *testing.T) {
	h := newHarness(t, "")
	h.exec("file.new one.txt")
	h.text("one")
	h.exec("view.split-vertically")
	h.exec("file.new two.txt")
	h.text("two")
	h.exec("view.previous")
	if filepath.Base(h.app.CurrentPath()) != "one.txt" || h.content() != "one" {
		t.Fatal("first pane lost buffer")
	}
	h.exec("view.next")
	if filepath.Base(h.app.CurrentPath()) != "two.txt" || h.content() != "two" {
		t.Fatal("second pane lost buffer")
	}
	h.mouse(5, 2, 0, false, false)
	h.mouse(5, 2, 0, false, true)
	if filepath.Base(h.app.CurrentPath()) != "one.txt" {
		t.Fatal("mouse did not focus first pane")
	}
}
func TestClosingSplitBufferCollapsesItsPane(t *testing.T) {
	h := newHarness(t, "")
	h.exec("file.new one.txt")
	h.exec("view.split-vertically")
	h.exec("file.new two.txt")
	h.exec("buffer.close")
	if filepath.Base(h.app.CurrentPath()) != "one.txt" {
		t.Fatal("wrong surviving buffer")
	}
	if err := h.app.Execute("view.next"); err == nil {
		t.Fatal("single remaining pane stayed split")
	}
}
func TestNestedViewCloseOnlyCollapsesFocusedBranch(t *testing.T) {
	h := newHarness(t, "")
	h.exec("view.split-vertically")
	h.exec("view.split-horizontally")
	h.exec("view.close")
	h.exec("view.next")
	h.exec("view.close")
	if err := h.app.Execute("view.next"); err == nil {
		t.Fatal("nested split did not collapse")
	}
}
func TestTerminalResizeReflowsEditor(t *testing.T) {
	h := newHarness(t, "")
	h.replace(strings.Repeat("x", 100))
	h.session.width = 60
	h.session.height = 12
	h.app.Update()
	h.draw()
	h.exec("cursor.line-end")
	if h.app.CurrentDocument().Cursor.Column != 100 {
		t.Fatal("resize broke navigation")
	}
	h.key(terminal.Event{Key: terminal.KeyLeft, Shift: true})
	if h.selection() != "x" {
		t.Fatal("resize broke selection")
	}
}
