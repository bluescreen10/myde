package golang_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bluescreen10/myde/plugin"
	golangplugin "github.com/bluescreen10/myde/plugins/golang"
	"github.com/bluescreen10/myde/ui"
)

type testHost struct {
	commands    map[string]plugin.Command
	document    plugin.Document
	modes       []plugin.Mode
	message     string
	settings    map[string]string
	subscribers map[string][]plugin.EventHandler
}

func (h *testHost) Root() string                     { return "" }
func (h *testHost) CurrentPath() string              { return h.document.Path }
func (h *testHost) CurrentDocument() plugin.Document { return h.document }
func (h *testHost) NewView(ui.View) ui.ViewHandle {
	return noopViewHandle{}
}
func (h *testHost) OpenSidebar(ui.Sidebar)                            {}
func (h *testHost) CloseSidebar()                                     {}
func (h *testHost) OpenReadOnlyBuffer(string, []byte)                 {}
func (h *testHost) OpenTextEditor(string, []byte, func([]byte) error) {}
func (h *testHost) Prompt(string, func(string) error)                 {}
func (h *testHost) SetMessage(message string)                         { h.message = message }
func (h *testHost) ReplaceCurrentDocument(content []byte) error {
	h.document.Content = append([]byte(nil), content...)
	h.document.Dirty = true
	return nil
}
func (h *testHost) Setting(name string) string { return h.settings[name] }
func (h *testHost) Subscribe(name string, handler plugin.EventHandler) error {
	if h.subscribers == nil {
		h.subscribers = make(map[string][]plugin.EventHandler)
	}
	h.subscribers[name] = append(h.subscribers[name], handler)
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
func (h *testHost) RegisterStatus(string, plugin.StatusItem) error { return nil }

type noopViewHandle struct{}

func (noopViewHandle) Show() bool          { return true }
func (noopViewHandle) Update(ui.View) bool { return true }
func (noopViewHandle) Destroy()            {}

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
	wantSubscribers := 0
	for _, command := range []string{"goimports", "gofmt"} {
		if _, err := exec.LookPath(command); err == nil {
			wantSubscribers++
		}
	}
	if got := len(host.subscribers[plugin.EventFileBeforeSave]); got != wantSubscribers {
		t.Fatalf("file.before-save subscribers = %d, want %d", got, wantSubscribers)
	}
}

func TestMissingFormattersDoNotRegisterSaveCallbacks(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	host := &testHost{commands: make(map[string]plugin.Command)}
	if err := golangplugin.New().Load(host); err != nil {
		t.Fatal(err)
	}
	if got := len(host.subscribers[plugin.EventFileBeforeSave]); got != 0 {
		t.Fatalf("registered %d save callbacks without formatter executables", got)
	}
}

func TestFormatOnSaveUsesSettings(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt is not installed")
	}
	host := &testHost{
		commands: make(map[string]plugin.Command),
		settings: map[string]string{"go-imports-on-save": "false"},
		document: plugin.Document{
			Path:    filepath.Join(t.TempDir(), "main.go"),
			Content: []byte("package main\nfunc main(){println(\"ok\")}\n"),
		},
	}
	if err := golangplugin.New().Load(host); err != nil {
		t.Fatal(err)
	}
	handlers := host.subscribers[plugin.EventFileBeforeSave]
	for _, handler := range handlers {
		if err := handler(plugin.Event{Name: plugin.EventFileBeforeSave, Value: host.document.Path}); err != nil {
			t.Fatal(err)
		}
	}
	want := []byte("package main\n\nfunc main() { println(\"ok\") }\n")
	if !bytes.Equal(host.document.Content, want) {
		t.Fatalf("formatted buffer = %q, want %q", host.document.Content, want)
	}

	host.settings["go-format-on-save"] = "false"
	host.document.Content = []byte("package main\nfunc main(){}\n")
	for _, handler := range handlers {
		if err := handler(plugin.Event{Name: plugin.EventFileBeforeSave, Value: host.document.Path}); err != nil {
			t.Fatal(err)
		}
	}
	if got := string(host.document.Content); got != "package main\nfunc main(){}\n" {
		t.Fatalf("disabled format changed buffer to %q", got)
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
