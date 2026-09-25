package golang_test

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/bluescreen10/myde/plugin"
	golangplugin "github.com/bluescreen10/myde/plugins/golang"
)

type testHost struct {
	commands map[string]plugin.Command
	document plugin.Document
	modes    []plugin.Mode
	message  string
}

func (h *testHost) Root() string                      { return "" }
func (h *testHost) CurrentPath() string               { return h.document.Path }
func (h *testHost) CurrentDocument() plugin.Document  { return h.document }
func (h *testHost) OpenSidebar(plugin.Sidebar)        {}
func (h *testHost) CloseSidebar()                     {}
func (h *testHost) OpenReadOnlyBuffer(string, []byte) {}
func (h *testHost) Prompt(string, func(string) error) {}
func (h *testHost) SetMessage(message string)         { h.message = message }
func (h *testHost) ReplaceCurrentDocument(content []byte) error {
	h.document.Content = append([]byte(nil), content...)
	h.document.Dirty = true
	return nil
}
func (h *testHost) RegisterCommand(name string, command plugin.Command) error {
	h.commands[name] = command
	return nil
}
func (h *testHost) RegisterMode(mode plugin.Mode) error {
	h.modes = append(h.modes, mode)
	return nil
}

func TestPluginRegistersGoModeAndCommands(t *testing.T) {
	host := &testHost{commands: make(map[string]plugin.Command)}
	if err := golangplugin.New().Load(host); err != nil {
		t.Fatal(err)
	}
	if len(host.modes) != 1 {
		t.Fatalf("registered %d modes, want 1", len(host.modes))
	}
	mode := host.modes[0]
	if mode.Name != "Go" || len(mode.Extensions) != 1 || mode.Extensions[0] != ".go" {
		t.Fatalf("registered mode = %#v", mode)
	}
	if mode.LanguageServer.Command != "gopls" {
		t.Fatalf("language server = %q, want gopls", mode.LanguageServer.Command)
	}
	if mode.DebugAdapter.Program.Command != "dlv" || mode.DebugAdapter.Transport != plugin.DebugReverseTCP {
		t.Fatalf("debug adapter = %#v", mode.DebugAdapter)
	}
	for _, name := range []string{"go.fmt", "go.vet", "go.build", "go.test"} {
		if host.commands[name] == nil {
			t.Fatalf("command %q was not registered", name)
		}
	}
}

func TestFormatReplacesCurrentBuffer(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt is not installed")
	}
	host := &testHost{
		commands: make(map[string]plugin.Command),
		document: plugin.Document{Content: []byte("package main\nfunc main(){println(\"ok\")}\n")},
	}
	if err := golangplugin.New().Load(host); err != nil {
		t.Fatal(err)
	}
	if err := host.commands["go.fmt"](""); err != nil {
		t.Fatal(err)
	}
	want := []byte("package main\n\nfunc main() { println(\"ok\") }\n")
	if !bytes.Equal(host.document.Content, want) {
		t.Fatalf("formatted buffer = %q, want %q", host.document.Content, want)
	}
}
