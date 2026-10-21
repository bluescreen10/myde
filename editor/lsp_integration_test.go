package editor_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	golangplugin "github.com/bluescreen10/myde/plugins/golang"
	"github.com/bluescreen10/myde/terminal"
)

func TestGoplsDefinitionAndRestart(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls is not installed")
	}
	root := t.TempDir()
	for name, content := range map[string]string{"go.mod": "module example.com/lspcheck\n\ngo 1.23\n", "main.go": "package main\n\nfunc target() {}\n\nfunc main() { target() }\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, root, golangplugin.New())
	h.exec("file.open main.go")
	definition := func() {
		t.Helper()
		h.exec("cursor.goto-line 5")
		for range 15 {
			h.key(terminal.Event{Key: terminal.KeyRight})
		}
		h.exec("lsp.definition")
		h.wait("Go definition", func() bool { return h.app.CurrentDocument().Cursor.Line == 2 })
	}
	definition()
	h.exec("go.lsp.restart")
	definition()
	h.exec("go.lsp.stop")
	if err := h.app.Execute("lsp.definition"); err == nil {
		t.Fatal("stopped Go server restarted")
	}
	h.exec("go.lsp.start")
	definition()
}

func TestGoplsWorkspaceCaseMismatch(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS path spelling")
	}
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls is not installed")
	}
	for _, subdirectory := range []string{"", "ui/win32"} {
		t.Run(subdirectory, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "gamekit", subdirectory)
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			inputRoot := filepath.Join(parent, "GameKit", subdirectory)
			if _, err := os.Stat(inputRoot); err != nil {
				t.Skip("filesystem is case sensitive")
			}
			for name, content := range map[string]string{
				"go.mod":  "module example.com/casecheck\n\ngo 1.23\n",
				"main.go": "package main\n\nfunc target() {}\n\nfunc main() { target() }\n",
			} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			h := newHarness(t, inputRoot, golangplugin.New())
			if got := h.app.Root(); got != root {
				t.Fatalf("workspace root = %q, want %q", got, root)
			}
			h.exec("file.open " + filepath.Join(inputRoot, "MAIN.go"))
			if got, want := h.app.CurrentPath(), filepath.Join(root, "main.go"); got != want {
				t.Fatalf("document path = %q, want %q", got, want)
			}
			h.exec("cursor.goto-line 5")
			for range 15 {
				h.key(terminal.Event{Key: terminal.KeyRight})
			}
			h.exec("lsp.definition")
			h.wait("Go definition with corrected path spelling", func() bool { return h.app.CurrentDocument().Cursor.Line == 2 })
			h.exec("file.open " + filepath.Join(root, "main.go"))
			if got := len(h.app.OpenDocuments()); got != 1 {
				t.Fatalf("opening both spellings created %d documents", got)
			}
		})
	}
}
