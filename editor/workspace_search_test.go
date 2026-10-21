package editor_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluescreen10/myde/editor"
	searchplugin "github.com/bluescreen10/myde/plugins/search"
	"github.com/bluescreen10/myde/terminal"
)

func TestSearchTabPreviewsLocationsAndReusesItsTab(t *testing.T) {
	h := newHarness(t, "")
	lines := make([]string, 90)
	for index := range lines {
		lines[index] = fmt.Sprintf("context row %d", index+1)
	}
	lines[35], lines[70] = "needle FIRST", "needle SECOND"
	path := filepath.Join(h.root, "source.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	h.exec("search.project needle")
	h.wait("first source preview", func() bool { return strings.Contains(h.screen.row(1), "source.txt:36") && h.contains("context row 35") })
	if h.app.CurrentPath() != "" || !h.app.CurrentDocument().ReadOnly {
		t.Fatal("preview opened the source file for editing")
	}
	for range 3 {
		h.key(terminal.Event{Key: terminal.KeyRune, Rune: 'f', Control: true, Shift: true})
	}
	if strings.Count(h.screen.row(0), ":Search") != 1 {
		t.Fatalf("search created duplicate tabs: %q", h.screen.row(0))
	}
	h.key(terminal.Event{Key: terminal.KeyDown})
	h.wait("second source preview", func() bool { return strings.Contains(h.screen.row(1), "source.txt:71") && h.contains("context row 70") })
	// The preview is centered on the selected match, without a line-number
	// gutter in the text copied by mouse selection.
	h.mouse(33, 11, 0, false, false)
	h.mouse(39, 11, 0, true, false)
	h.mouse(39, 11, 0, false, true)
	if h.selection() != "needle" {
		t.Fatalf("preview selection = %q", h.selection())
	}
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if h.app.CurrentPath() != path || h.app.CurrentDocument().Cursor.Line != 70 {
		t.Fatal("Enter did not open the selected match for editing")
	}
	h.key(terminal.Event{Key: terminal.KeyRune, Rune: 'f', Control: true, Shift: true})
	h.wait("restored preview", func() bool { return strings.Contains(h.screen.row(1), "source.txt:71") })
	if strings.Count(h.screen.row(0), ":Search") != 1 {
		t.Fatal("raising search from a source file created another tab")
	}
	// Clicking another result also moves the preview and returns query focus.
	h.mouse(3, 5, 0, false, false)
	h.mouse(3, 5, 0, false, true)
	h.wait("clicked source preview", func() bool { return strings.Contains(h.screen.row(1), "source.txt:36") })
	h.key(terminal.Event{Key: terminal.KeyTab})
	h.key(terminal.Event{Key: terminal.KeyDown})
	if !strings.Contains(h.screen.row(2), "context row 28") {
		t.Fatalf("preview did not scroll: %q", h.screen.row(2))
	}
	h.key(terminal.Event{Key: terminal.KeyTab})
	h.key(terminal.Event{Key: terminal.KeyLeft, Control: true})
	h.text("missing ")
	h.wait("empty results clear preview", func() bool { return h.contains("No matches") && !strings.Contains(h.screen.row(1), "source.txt:") })
}

func TestWorkspaceSearchIsProvidedByPlugin(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MYDE_CONFIG", filepath.Join(root, ".myde"))
	app, err := editor.NewWithSettings(root, nil, &testSession{100, 24}, strings.NewReader(""), io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if err := app.Execute("search.project"); err == nil {
		t.Fatal("editor core registered the plugin's command")
	}
	p := searchplugin.New()
	if err := p.Load(app); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.(io.Closer).Close() })
	if err := app.Execute("search.project"); err != nil {
		t.Fatalf("search plugin did not register its command: %v", err)
	}
}

func TestWorkspaceSearchIsRecursiveAndSmartCase(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		name := "ripgrep"
		if fallback {
			name = "fallback"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, "")
			if fallback {
				t.Setenv("PATH", t.TempDir())
			}
			if err := os.Mkdir(filepath.Join(h.root, "nested"), 0700); err != nil {
				t.Fatal(err)
			}
			for name, text := range map[string]string{"a.txt": "first Needle\n", "nested/b.txt": "another needle\n", "binary": "n\x00needle"} {
				if err := os.WriteFile(filepath.Join(h.root, name), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			h.app.Update()
			h.exec("search.project needle")
			h.wait("recursive results", func() bool { return h.contains("another needle") && h.contains("first Needle") })
			h.key(terminal.Event{Key: terminal.KeyEscape})
			h.exec("search.project Needle")
			h.wait("smart-case results", func() bool { return h.contains("first Needle") && !h.contains("another needle") })
			if h.contains("binary") {
				t.Fatal("binary was searched")
			}
		})
	}
}
