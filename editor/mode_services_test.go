package editor_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/protocol"
	"github.com/bluescreen10/myde/terminal"
)

type toolRecord struct {
	PID     int
	Message protocol.Message
}

// This subprocess speaks the public wire protocols, allowing independent
// plugin processes and routing to be tested without accessing editor state.
func TestModeToolHelper(t *testing.T) {
	index := slices.Index(os.Args, "myde-tool-helper")
	if index < 0 {
		return
	}
	arguments := os.Args[index+1:]
	if len(arguments) < 3 {
		os.Exit(2)
	}
	kind, name, path := arguments[0], arguments[1], arguments[2]
	log, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		os.Exit(2)
	}
	stream := protocol.NewStream(os.Stdin, os.Stdout)
	if kind == "dap-reverse" {
		if len(arguments) < 4 {
			os.Exit(5)
		}
		connection, err := net.Dial("tcp", arguments[3])
		if err != nil {
			os.Exit(6)
		}
		stream = protocol.NewStream(connection, connection)
	}
	write := func(message any) {
		if stream.Write(message) != nil {
			os.Exit(3)
		}
	}
	for {
		message, err := stream.Read()
		if err != nil {
			os.Exit(0)
		}
		entry, _ := json.Marshal(toolRecord{PID: os.Getpid(), Message: message})
		if _, err := log.Write(append(entry, '\n')); err != nil {
			os.Exit(4)
		}
		if kind == "lsp" {
			if len(message.ID) == 0 {
				if message.Method == "textDocument/didOpen" {
					if name == "workspace-error" {
						write(map[string]any{"jsonrpc": "2.0", "method": "window/logMessage", "params": map[string]any{"type": 3, "message": "Starting workspace load"}})
						write(map[string]any{"jsonrpc": "2.0", "method": "window/showMessage", "params": map[string]any{"type": 1, "message": "go command required, not found\nfailed to load workspace view"}})
						continue
					}
					var params struct {
						TextDocument struct {
							URI string `json:"uri"`
						} `json:"textDocument"`
					}
					_ = json.Unmarshal(message.Params, &params)
					write(map[string]any{"jsonrpc": "2.0", "method": "textDocument/publishDiagnostics", "params": map[string]any{"uri": params.TextDocument.URI, "diagnostics": []any{map[string]any{"range": map[string]any{"start": map[string]int{"line": 0, "character": 0}, "end": map[string]int{"line": 0, "character": 1}}, "severity": 1, "message": name + " diagnostics"}}}})
				}
				continue
			}
			if name == "workspace-error" && (message.Method == "textDocument/definition" || message.Method == "textDocument/completion") {
				write(map[string]any{"jsonrpc": "2.0", "id": message.ID, "error": map[string]any{"code": 0, "message": "no views"}})
				continue
			}
			var result any
			switch message.Method {
			case "initialize":
				sync := 1
				if name != "Go" {
					sync = 2
				}
				result = map[string]any{"capabilities": map[string]any{"textDocumentSync": sync, "completionProvider": map[string]any{"resolveProvider": name == "TypeScript"}}}
			case "textDocument/definition":
				var params struct {
					TextDocument struct {
						URI string `json:"uri"`
					} `json:"textDocument"`
				}
				_ = json.Unmarshal(message.Params, &params)
				result = []any{map[string]any{"uri": params.TextDocument.URI, "range": map[string]any{"start": map[string]int{"line": 1, "character": 0}, "end": map[string]int{"line": 1, "character": 1}}}}
			case "textDocument/completion":
				result = []any{map[string]any{"label": name + "Result", "data": map[string]string{"mode": name}}}
			case "completionItem/resolve":
				result = map[string]any{"label": name + "Result", "detail": name + " resolved"}
			}
			write(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
		} else {
			if message.Command == "launch" {
				write(map[string]any{"seq": 1, "type": "event", "event": "initialized"})
			}
			write(map[string]any{"seq": 2, "type": "response", "request_seq": message.Sequence, "command": message.Command, "success": true, "body": map[string]any{}})
			if message.Command == "disconnect" {
				os.Exit(0)
			}
			if message.Command == "next" || message.Command == "continue" {
				write(map[string]any{"seq": 3, "type": "event", "event": "output", "body": map[string]string{"output": name + " " + message.Command}})
			}
		}
	}
}

type toolPlugin struct{ directory, executable string }

func (p toolPlugin) Name() string { return "test-tools" }
func (p toolPlugin) program(kind, name string) plugin.Program {
	return plugin.Program{Command: p.executable, Arguments: []string{"-test.run=^TestModeToolHelper$", "--", "myde-tool-helper", kind, name, filepath.Join(p.directory, name+"-"+kind+".log")}}
}
func (p toolPlugin) Load(host plugin.Host) error {
	for _, mode := range []struct{ name, extension, id string }{{"Go", ".go", "go"}, {"TypeScript", ".ts", "typescript"}, {"Rust", ".rs", "rust"}} {
		if err := host.RegisterMode(plugin.Mode{Name: mode.name, Extensions: []string{mode.extension}, Syntax: "plain", LanguageID: mode.id, LanguageServer: p.program("lsp", mode.name), DebugAdapter: plugin.DebugAdapter{Program: p.program("dap", mode.name), Transport: plugin.DebugStandardIO}}); err != nil {
			return err
		}
	}
	return nil
}
func toolHarness(t *testing.T) (*harness, toolPlugin) {
	t.Helper()
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := toolPlugin{t.TempDir(), exe}
	for name, text := range map[string]string{"a.go": "go source\ngo target", "b.go": "second Go file", "a.ts": "ts source\nts target", "a.rs": "rust source\nrust target"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return newHarness(t, root, p), p
}
func (p toolPlugin) records(t *testing.T, name, kind string) []toolRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(p.directory, name+"-"+kind+".log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var records []toolRecord
	for _, line := range strings.Split(string(data), "\n") {
		var record toolRecord
		if json.Unmarshal([]byte(line), &record) == nil {
			records = append(records, record)
		}
	}
	return records
}
func (p toolPlugin) messages(t *testing.T, name, kind, method string) []protocol.Message {
	t.Helper()
	var messages []protocol.Message
	for _, record := range p.records(t, name, kind) {
		if record.Message.Method == method || record.Message.Command == method {
			messages = append(messages, record.Message)
		}
	}
	return messages
}
func (p toolPlugin) pid(t *testing.T, name, kind string) int {
	t.Helper()
	records := p.records(t, name, kind)
	if len(records) == 0 {
		return 0
	}
	return records[len(records)-1].PID
}

func TestModeLanguageServersRunConcurrentlyAndHaveIndependentLifecycle(t *testing.T) {
	h, p := toolHarness(t)
	for _, path := range []string{"a.go", "a.ts", "a.rs", "b.go"} {
		h.exec("file.open " + path)
	}
	for _, name := range []string{"Go", "TypeScript", "Rust"} {
		if len(p.messages(t, name, "lsp", "initialize")) != 1 {
			t.Fatalf("%s was not started once", name)
		}
	}
	h.wait("both Go documents", func() bool { return len(p.messages(t, "Go", "lsp", "textDocument/didOpen")) == 2 })
	goPID, tsPID, rustPID := p.pid(t, "Go", "lsp"), p.pid(t, "TypeScript", "lsp"), p.pid(t, "Rust", "lsp")
	if goPID == tsPID || goPID == rustPID || tsPID == rustPID {
		t.Fatal("modes did not get independent processes")
	}
	if err := h.app.StartLanguageServer("Go"); err != nil {
		t.Fatal(err)
	}
	if p.pid(t, "Go", "lsp") != goPID {
		t.Fatal("start did not reuse server")
	}
	h.exec("file.open a.ts")
	if err := h.app.StopLanguageServer("Go"); err != nil {
		t.Fatal(err)
	}
	h.exec("file.open a.go")
	if err := h.app.Execute("lsp.definition"); err == nil {
		t.Fatal("stopped server automatically restarted")
	}
	if len(p.messages(t, "Go", "lsp", "initialize")) != 1 {
		t.Fatal("stopped mode restarted on activation")
	}
	h.app.Update()
	h.draw()
	if h.contains("Go diagnostics") {
		t.Fatal("stopped server left diagnostics behind")
	}
	if err := h.app.StartLanguageServer("Go"); err != nil {
		t.Fatal(err)
	}
	if p.pid(t, "Go", "lsp") == goPID {
		t.Fatal("start reused stopped process")
	}
	if err := h.app.RestartLanguageServer("TypeScript"); err != nil {
		t.Fatal(err)
	}
	if p.pid(t, "TypeScript", "lsp") == tsPID {
		t.Fatal("restart reused old process")
	}
	if p.pid(t, "Rust", "lsp") != rustPID || len(p.messages(t, "Rust", "lsp", "initialize")) != 1 {
		t.Fatal("other mode was restarted")
	}
	for _, path := range []string{"a.go", "a.ts", "a.rs"} {
		h.exec("file.open " + path)
		h.exec("cursor.goto-line 1")
		h.exec("lsp.definition")
		h.wait("mode-specific definition", func() bool { return h.app.CurrentDocument().Cursor.Line == 1 })
	}
}

func TestModeServersKeepCapabilitiesAndDocumentRoutingSeparate(t *testing.T) {
	h, p := toolHarness(t)
	h.exec("file.open a.go")
	h.exec("file.open a.ts")
	h.exec("file.open a.go")
	h.replace("updated Go\ngo target")
	h.exec("file.open a.ts")
	h.text("x")
	h.wait("changes on each server", func() bool {
		return len(p.messages(t, "Go", "lsp", "textDocument/didChange")) > 0 && len(p.messages(t, "TypeScript", "lsp", "textDocument/didChange")) > 0
	})
	for _, name := range []string{"Go", "TypeScript"} {
		messages := p.messages(t, name, "lsp", "textDocument/didChange")
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []map[string]json.RawMessage `json:"contentChanges"`
		}
		if err := json.Unmarshal(messages[len(messages)-1].Params, &params); err != nil {
			t.Fatal(err)
		}
		suffix := "a.go"
		if name == "TypeScript" {
			suffix = "a.ts"
		}
		if !strings.HasSuffix(params.TextDocument.URI, suffix) {
			t.Fatalf("%s received %s", name, params.TextDocument.URI)
		}
		_, incremental := params.ContentChanges[0]["range"]
		if incremental != (name == "TypeScript") {
			t.Fatalf("%s used wrong sync capability: %s", name, messages[len(messages)-1].Params)
		}
	}
	h.exec("completion.show")
	h.wait("TypeScript completion resolution", func() bool { return h.contains("TypeScript resolved") })
	if len(p.messages(t, "Go", "lsp", "completionItem/resolve")) != 0 {
		t.Fatal("completion resolved on wrong server")
	}
	h.key(terminal.Event{Key: terminal.KeyEscape})
	h.exec("file.open a.go")
	h.exec("completion.show")
	h.wait("Go completion", func() bool { return h.contains("GoResult") })
	if len(p.messages(t, "Go", "lsp", "completionItem/resolve")) != 0 {
		t.Fatal("server without resolve capability received resolve")
	}
	h.key(terminal.Event{Key: terminal.KeyEscape})
	h.exec("switch.mode TypeScript")
	h.wait("mode reassignment", func() bool {
		return len(p.messages(t, "Go", "lsp", "textDocument/didClose")) > 0 && len(p.messages(t, "TypeScript", "lsp", "textDocument/didOpen")) == 2
	})
	if h.app.CurrentDocument().Mode != "TypeScript" {
		t.Fatal("mode reassignment failed")
	}
}

func TestModeDebuggersHaveIndependentLifecycleAndBreakpointRouting(t *testing.T) {
	h, p := toolHarness(t)
	for _, name := range []string{"Go", "TypeScript", "Rust"} {
		if err := h.app.StartDebugAdapter(name); err != nil {
			t.Fatal(err)
		}
	}
	goPID, tsPID := p.pid(t, "Go", "dap"), p.pid(t, "TypeScript", "dap")
	if err := h.app.StartDebugAdapter("Go"); err != nil {
		t.Fatal(err)
	}
	if p.pid(t, "Go", "dap") != goPID {
		t.Fatal("start did not reuse debugger")
	}
	h.exec("file.open a.go")
	h.exec("debug.toggle-breakpoint")
	h.exec("file.open a.ts")
	h.exec("debug.toggle-breakpoint")
	if err := h.app.LaunchDebugProgram("Go", "go-program"); err != nil {
		t.Fatal(err)
	}
	if err := h.app.LaunchDebugProgram("TypeScript", "ts-program"); err != nil {
		t.Fatal(err)
	}
	h.wait("both debugger configurations", func() bool {
		return len(p.messages(t, "Go", "dap", "configurationDone")) > 0 && len(p.messages(t, "TypeScript", "dap", "configurationDone")) > 0
	})
	for _, name := range []string{"Go", "TypeScript"} {
		for _, message := range p.messages(t, name, "dap", "setBreakpoints") {
			var args struct {
				Source struct {
					Path string `json:"path"`
				} `json:"source"`
			}
			_ = json.Unmarshal(message.Arguments, &args)
			want := "a.go"
			if name == "TypeScript" {
				want = "a.ts"
			}
			if filepath.Base(args.Source.Path) != want {
				t.Fatalf("%s received other mode's breakpoints: %s", name, args.Source.Path)
			}
		}
	}
	h.exec("debug.next")
	h.wait("active mode debugger step", func() bool { return len(p.messages(t, "TypeScript", "dap", "next")) > 0 })
	if len(p.messages(t, "Go", "dap", "next")) != 0 {
		t.Fatal("stepped wrong debugger")
	}
	if err := h.app.StopDebugAdapter("Go"); err != nil {
		t.Fatal(err)
	}
	if len(p.messages(t, "Go", "dap", "disconnect")) != 1 || len(p.messages(t, "TypeScript", "dap", "disconnect")) != 0 {
		t.Fatal("stop affected wrong mode")
	}
	h.exec("debug.continue")
	h.wait("surviving debugger", func() bool { return len(p.messages(t, "TypeScript", "dap", "continue")) > 0 })
	if err := h.app.RestartDebugAdapter("Go"); err != nil {
		t.Fatal(err)
	}
	if p.pid(t, "Go", "dap") == goPID || p.pid(t, "TypeScript", "dap") != tsPID {
		t.Fatal("restart affected wrong debugger")
	}
}

func TestModeServiceErrorsDoNotStopOtherModes(t *testing.T) {
	h, p := toolHarness(t)
	h.exec("file.open a.go")
	if err := h.app.StartDebugAdapter("Go"); err != nil {
		t.Fatal(err)
	}
	if err := h.app.RunLanguageServer("TypeScript", plugin.Program{Command: "/missing/myde-server"}); err == nil {
		t.Fatal("missing server accepted")
	}
	if err := h.app.RunDebugAdapter("TypeScript", plugin.DebugAdapter{Program: plugin.Program{Command: "/missing/myde-debugger"}}); err == nil {
		t.Fatal("missing debugger accepted")
	}
	if err := h.app.StopLanguageServer("unknown"); err == nil {
		t.Fatal("unknown mode accepted")
	}
	h.exec("debug.continue")
	h.wait("Go debugger still alive", func() bool { return len(p.messages(t, "Go", "dap", "continue")) > 0 })
	h.exec("lsp.definition")
	h.wait("Go server still alive", func() bool { return h.app.CurrentDocument().Cursor.Line == 1 })
}

func TestLanguageServerWorkspaceErrorsRemainVisibleAndHaveReadableLogs(t *testing.T) {
	h, p := toolHarness(t)
	h.exec("file.open a.go")
	if err := h.app.RunLanguageServer("Go", p.program("lsp", "workspace-error")); err != nil {
		t.Fatal(err)
	}
	h.wait("workspace failure", func() bool { return h.contains("go command required, not found") })
	for _, command := range []string{"lsp.definition", "completion.show"} {
		h.app.SetMessage("")
		h.exec(command)
		method := "textDocument/definition"
		if command == "completion.show" {
			method = "textDocument/completion"
		}
		h.wait(command+" response", func() bool {
			return len(p.messages(t, "workspace-error", "lsp", method)) > 0 && h.contains("lsp.log")
		})
		if !h.contains("go command required, not found") {
			t.Fatal("request error hid the workspace failure")
		}
	}
	h.exec("file.open a.ts")
	h.exec("lsp.log Go")
	for _, want := range []string{"Workspace: " + h.root, "Server: " + p.executable, "Starting workspace load", "go command required, not found\nfailed to load workspace view", "textDocument/definition", "textDocument/completion", "no views"} {
		if !strings.Contains(h.content(), want) {
			t.Fatalf("language server log missing %q: %s", want, h.content())
		}
	}
	if err := h.app.ReplaceCurrentDocument([]byte("edit")); err == nil {
		t.Fatal("language server log is editable")
	}
	h.exec("file.open a.go")
	if err := h.app.RunLanguageServer("Go", p.program("lsp", "recovered")); err != nil {
		t.Fatal(err)
	}
	h.exec("lsp.definition")
	h.wait("recovered definition", func() bool { return h.app.CurrentDocument().Cursor.Line == 1 })
	h.exec("lsp.log")
	if strings.Contains(h.content(), "go command required") {
		t.Fatal("new server retained the previous session's failure")
	}
}

func TestModeServiceRunUsesExplicitPrograms(t *testing.T) {
	h, p := toolHarness(t)
	if err := h.app.RunLanguageServer("Go", p.program("lsp", "custom")); err != nil {
		t.Fatal(err)
	}
	if err := h.app.RunDebugAdapter("Go", plugin.DebugAdapter{Program: p.program("dap", "custom"), Transport: plugin.DebugStandardIO}); err != nil {
		t.Fatal(err)
	}
	h.exec("file.open a.go")
	h.exec("lsp.definition")
	h.wait("explicit server", func() bool { return h.app.CurrentDocument().Cursor.Line == 1 })
	h.exec("debug.continue")
	h.wait("explicit debugger", func() bool { return len(p.messages(t, "custom", "dap", "continue")) > 0 })
	if err := h.app.RestartLanguageServer("Go"); err != nil {
		t.Fatal(err)
	}
	if err := h.app.RestartDebugAdapter("Go"); err != nil {
		t.Fatal(err)
	}
	if len(p.messages(t, "custom", "lsp", "initialize")) != 2 || len(p.messages(t, "custom", "dap", "initialize")) != 2 {
		t.Fatal("restart lost explicit programs")
	}
	if len(p.messages(t, "custom", "dap", "disconnect")) != 1 {
		t.Fatal("restart did not disconnect the previous debuggee")
	}

	if err := h.app.Close(); err != nil {
		t.Fatal(err)
	}
	if err := h.app.Execute("debug.continue"); err == nil {
		t.Fatal("closed debugger still active")
	}
}

func TestPluginsCanReplaceFallbackModesWithoutStealingPluginModes(t *testing.T) {
	h, _ := toolHarness(t)
	h.exec("file.open a.ts")
	if h.app.CurrentDocument().Mode != "TypeScript" {
		t.Fatal("TypeScript plugin did not claim .ts")
	}
	h.exec("file.open a.rs")
	if h.app.CurrentDocument().Mode != "Rust" {
		t.Fatal("Rust plugin did not replace fallback")
	}
	h.exec("file.new remaining.js")
	if h.app.CurrentDocument().Mode != "JavaScript" {
		t.Fatal("TypeScript plugin removed JavaScript fallback")
	}
	if err := h.app.RegisterMode(plugin.Mode{Name: "Rust", Extensions: []string{".rs"}}); err == nil {
		t.Fatal("duplicate plugin mode accepted")
	}
	if err := h.app.RegisterMode(plugin.Mode{Name: "Other", Extensions: []string{".ts"}}); err == nil {
		t.Fatal("plugin-owned extension stolen")
	}
}

func TestModeDebugAdapterSupportsReverseTCPAlongsideStandardIO(t *testing.T) {
	h, p := toolHarness(t)
	if err := h.app.StartDebugAdapter("TypeScript"); err != nil {
		t.Fatal(err)
	}
	program := p.program("dap-reverse", "reverse")
	program.Arguments = append(program.Arguments, "{address}")
	if err := h.app.RunDebugAdapter("Go", plugin.DebugAdapter{Program: program, Transport: plugin.DebugReverseTCP}); err != nil {
		t.Fatal(err)
	}
	if err := h.app.LaunchDebugProgram("Go", "go-program"); err != nil {
		t.Fatal(err)
	}
	h.wait("reverse debugger configuration", func() bool { return len(p.messages(t, "reverse", "dap-reverse", "configurationDone")) == 1 })
	if err := h.app.StopDebugAdapter("Go"); err != nil {
		t.Fatal(err)
	}
	if err := h.app.LaunchDebugProgram("TypeScript", "typescript-program"); err != nil {
		t.Fatal(err)
	}
	h.wait("standard I/O debugger still alive", func() bool { return len(p.messages(t, "TypeScript", "dap", "configurationDone")) == 1 })
}

func TestSwitchingModeClearsOldCompletionUI(t *testing.T) {
	h, _ := toolHarness(t)
	h.exec("file.open a.ts")
	h.exec("file.open a.go")
	h.exec("completion.show")
	h.wait("Go completion", func() bool { return h.contains("GoResult") })
	h.exec("switch.mode TypeScript")
	h.key(terminal.Event{Key: terminal.KeyRight})
	h.key(terminal.Event{Key: terminal.KeyLeft})
	h.wait("new mode diagnostics", func() bool { return h.contains("TypeScript diagnostics") })
	if h.contains("GoResult") {
		t.Fatal("old mode's completion remained visible")
	}
	h.exec("completion.show")
	h.wait("new mode completion", func() bool { return h.contains("TypeScriptResult") })
}
