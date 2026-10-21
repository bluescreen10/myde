package editor_test

import (
	"os"
	"os/exec"
	"path/filepath"
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
