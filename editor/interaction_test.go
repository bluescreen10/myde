package editor_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

func TestDefaultPageBindingsAndCommands(t *testing.T) {
	h := newHarness(t, "")
	h.replace(strings.Repeat("line\n", 60))
	h.exec("cursor.goto-line 40")
	h.key(terminal.Event{Key: terminal.KeyUp, Alt: true, Shift: true})
	if h.app.CurrentDocument().Cursor.Line >= 39 {
		t.Fatal("page-up binding did not move")
	}
	h.key(terminal.Event{Key: terminal.KeyDown, Alt: true, Shift: true})
	if h.app.CurrentDocument().Cursor.Line != 39 {
		t.Fatal("page-down did not restore line")
	}
}

func TestModifiedNavigationShortcutExtendsSelection(t *testing.T) {
	h := newHarness(t, "")
	h.replace("alpha")
	h.key(terminal.Event{Key: terminal.KeyRight})
	h.key(terminal.Event{Key: terminal.KeyRight})
	h.key(terminal.Event{Key: terminal.KeyRight, Control: true, Shift: true})
	if h.selection() != "pha" {
		t.Fatalf("selection=%q", h.selection())
	}
}

func TestWorkspaceChangesRefreshCommandPalette(t *testing.T) {
	h := newHarness(t, "")
	h.exec("command.palette")
	if err := os.Mkdir(filepath.Join(h.root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, "nested/new.go"), []byte("package nested"), 0600); err != nil {
		t.Fatal(err)
	}
	h.wait("new indexed file", func() bool { return h.contains("nested/new.go") })
	h.text("new.go\n")
	if filepath.Base(h.app.CurrentPath()) != "new.go" {
		t.Fatal("palette did not open new file")
	}
}

func TestCommandPaletteOrdersRecentItemsFirst(t *testing.T) {
	h := newHarness(t, "")
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(h.root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	h.exec("file.open a.txt")
	h.exec("file.open c.txt")
	h.exec("command.palette")
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if filepath.Base(h.app.CurrentPath()) != "c.txt" {
		t.Fatal("recent file was not first")
	}
}

func TestAddNextMatchAndEscapeCursorLayers(t *testing.T) {
	h := newHarness(t, "")
	h.replace("word x word")
	for range 4 {
		h.key(terminal.Event{Key: terminal.KeyRight, Shift: true})
	}
	h.exec("cursor.add-next-match")
	if h.selection() != "word\nword" {
		t.Fatalf("selections=%q", h.selection())
	}
	h.exec("cursor.add-next-match")
	if h.selection() != "word\nword" {
		t.Fatal("duplicate selection added")
	}
	h.key(terminal.Event{Key: terminal.KeyEscape})
	if h.selection() != "word" {
		t.Fatalf("first escape=%q", h.selection())
	}
	h.key(terminal.Event{Key: terminal.KeyEscape})
	if h.selection() != "" {
		t.Fatal("second Escape kept selection")
	}
}

func TestMultipleCursorsEditBothSourceLocations(t *testing.T) {
	h := newHarness(t, "")
	h.replace("word x word")
	for range 4 {
		h.key(terminal.Event{Key: terminal.KeyRight, Shift: true})
	}
	h.exec("cursor.add-next-match")
	h.text("new")
	if h.content() != "new x new" {
		t.Fatalf("multi-cursor edit=%q", h.content())
	}
}

func TestSidebarRefreshPreservesSelection(t *testing.T) {
	h := newHarness(t, "")
	chosen := ""
	items := []ui.SidebarItem{{Label: "a", Value: "a"}, {Label: "b", Value: "b"}}
	updated := ui.Sidebar{Title: "REFRESHED", Sections: []ui.SidebarSection{{Title: "Files", Items: items}}, OnAction: func(_ ui.Action, item ui.SidebarItem) error { chosen = item.Value; return nil }}
	original := updated
	original.Title = "Original"
	original.RefreshInterval = time.Millisecond
	original.OnRefresh = func() (ui.Sidebar, error) { return updated, nil }
	h.app.OpenSidebar(original)
	h.key(terminal.Event{Key: terminal.KeyDown})
	h.wait("sidebar refresh", func() bool { return h.contains("REFRESHED") })
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if chosen != "b" {
		t.Fatalf("selection=%q", chosen)
	}
}

func TestTypingGroupsUndoWordsAndNavigationFinishesGroups(t *testing.T) {
	h := newHarness(t, "")
	h.text("first second")
	h.exec("edit.undo")
	if h.content() != "first " {
		t.Fatalf("word undo=%q", h.content())
	}
	h.exec("edit.redo")
	h.key(terminal.Event{Key: terminal.KeyLeft})
	h.text("x")
	h.exec("edit.undo")
	if h.content() != "first second" {
		t.Fatalf("navigation undo=%q", h.content())
	}
}

func TestSavePreservesScrollState(t *testing.T) {
	h := newHarness(t, "")
	h.exec("file.new file.txt")
	h.replace(strings.Repeat("line\n", 50))
	h.exec("cursor.goto-line 40")
	h.exec("file.save")
	if h.app.CurrentDocument().Cursor.Line != 39 || !strings.Contains(h.screen.row(22), "line") {
		t.Fatalf("save lost viewport:\n%s", h.screen.text())
	}
}

func TestPluginTextEditorSubmitsMultilineContentOnSave(t *testing.T) {
	h := newHarness(t, "")
	fail := true
	submitted := ""
	h.app.OpenTextEditor("COMMIT_EDITMSG", nil, func(content []byte) error {
		if fail {
			return errors.New("commit failed")
		}
		submitted = string(content)
		return nil
	})
	h.text("subject\n\nbody\n")
	if err := h.app.Execute("file.save"); err == nil {
		t.Fatal("accepted failed submission")
	}
	if h.content() != "subject\n\nbody\n" {
		t.Fatal("failed submission lost content")
	}
	fail = false
	h.exec("file.save")
	if submitted != "subject\n\nbody\n" || strings.Contains(h.screen.row(0), "COMMIT_EDITMSG") {
		t.Fatal("successful submission did not close editor")
	}
}

func TestLocalCompletionTypingAndArrowNavigation(t *testing.T) {
	h := newHarness(t, "")
	h.replace("alpha alphabet\nal")
	h.exec("cursor.file-end")
	h.exec("completion.show")
	if !h.contains("Complete") {
		t.Fatal("local completion did not open")
	}
	h.key(terminal.Event{Key: terminal.KeyDown})
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if !strings.HasSuffix(h.content(), "alphabet") {
		t.Fatalf("completion=%q", h.content())
	}
}
