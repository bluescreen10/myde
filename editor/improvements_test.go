package editor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

func TestNavigationEditsEveryTextInput(t *testing.T) {
	for _, kind := range []string{"palette", "minibuffer", "files", "search"} {
		t.Run(kind, func(t *testing.T) {
			h := newHarness(t, "")
			submitted := ""
			switch kind {
			case "palette":
				h.app.Prompt("Input", func(s string) error { submitted = s; return nil })
			case "minibuffer":
				h.exec("file.save")
			case "files":
				h.exec("view.files")
			case "search":
				h.exec("search.project")
			}
			h.text("a界c")
			h.key(terminal.Event{Key: terminal.KeyLeft})
			h.text("b")
			h.key(terminal.Event{Key: terminal.KeyDelete})
			h.key(terminal.Event{Key: terminal.KeyLeft, Control: true})
			h.text("x")
			h.key(terminal.Event{Key: terminal.KeyRight, Control: true})
			h.key(terminal.Event{Key: terminal.KeyBackspace})
			if !h.contains("xa界") {
				t.Fatalf("input navigation lost text:\n%s", h.screen.text())
			}
			if h.content() != "" {
				t.Fatal("input edited source buffer")
			}
			if kind == "palette" {
				h.key(terminal.Event{Key: terminal.KeyEnter})
				if submitted != "xa界" {
					t.Fatalf("submitted=%q", submitted)
				}
			}
		})
	}
}

func TestMouseSelectsOnlySourceTextWithTabsAndWideRunes(t *testing.T) {
	h := newHarness(t, "")
	h.replace("\t界abc\nsecond")
	h.mouse(7, 1, 0, false, false)
	h.mouse(11, 1, 0, true, false)
	h.mouse(11, 1, 0, false, true)
	if h.selection() != "界ab" {
		t.Fatalf("selection=%q", h.selection())
	}
	h.key(terminal.Event{Key: terminal.KeyEscape})
	if h.selection() != "" {
		t.Fatal("Escape retained selection")
	}
}

func TestGotoLinePromptAndClamping(t *testing.T) {
	h := newHarness(t, "")
	h.replace("a\nb\nc")
	h.exec("cursor.goto-line 2")
	if h.app.CurrentDocument().Cursor.Line != 1 {
		t.Fatal("wrong line")
	}
	h.exec("cursor.goto-line 999")
	if h.app.CurrentDocument().Cursor.Line != 2 {
		t.Fatal("did not clamp")
	}
	if err := h.app.Execute("cursor.goto-line 0"); err == nil {
		t.Fatal("accepted zero")
	}
	h.key(terminal.Event{Key: terminal.KeyRune, Rune: 'g', Control: true})
	h.text("1\n")
	if h.app.CurrentDocument().Cursor.Line != 0 {
		t.Fatal("prompt did not navigate")
	}
}

func TestSearchPersistsAndRescansWhileHidden(t *testing.T) {
	h := newHarness(t, "")
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(h.root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "needle\nneedle again\n")
	write("b.txt", "other\n")
	h.app.Update()
	h.exec("search.project needle")
	h.wait("two matches", func() bool { return h.contains("needle again") })
	h.key(terminal.Event{Key: terminal.KeyDown})
	h.key(terminal.Event{Key: terminal.KeyEscape})
	h.exec("search.project")
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if h.app.CurrentDocument().Cursor.Line != 1 {
		t.Fatal("search did not restore selected result")
	}
	write("a.txt", "gone\n")
	write("b.txt", "new needle match\n")
	h.app.Update()
	h.exec("search.project")
	h.wait("refreshed results", func() bool { return h.contains("new needle match") && !h.contains("needle again") })
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if filepath.Base(h.app.CurrentPath()) != "b.txt" {
		t.Fatal("opened stale match")
	}
}

func TestSearchRescansUnsavedEdits(t *testing.T) {
	h := newHarness(t, "")
	h.exec("file.new a.txt")
	h.replace("needle on disk\n")
	h.exec("file.save")
	h.exec("search.project needle")
	h.wait("saved match", func() bool { return h.contains("needle on disk") })
	h.key(terminal.Event{Key: terminal.KeyEnter})
	h.replace("unsaved needle\n")
	h.exec("search.project")
	h.wait("unsaved match", func() bool { return strings.Contains(h.screen.row(5), "unsaved needle") })
	h.key(terminal.Event{Key: terminal.KeyEnter})
	h.exec("edit.undo")
	h.exec("search.project")
	h.wait("restored match", func() bool { return strings.Contains(h.screen.row(5), "needle on disk") })
}

func TestSearchShortcutFromPluginViewAndPrompt(t *testing.T) {
	h := newHarness(t, "")
	h.app.NewView(ui.View{Title: "Git Stage", Layout: ui.Layout{Panes: []ui.Pane{{ID: "changes"}}}})
	h.app.Prompt("Other input", func(string) error { return nil })
	h.key(terminal.Event{Key: terminal.KeyRune, Rune: 'f', Control: true, Shift: true})
	h.text("test")
	if !h.contains("Search ·") || h.contains("Other input") {
		t.Fatalf("search did not take focus:\n%s", h.screen.text())
	}
}
