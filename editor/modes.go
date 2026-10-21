package editor

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/syntax"
)

// editingMode owns the runtime shared by all buffers using a plugin's mode.
type editingMode struct {
	plugin.Mode
	builtin        bool
	languageServer *languageServerSession
	serverProgram  plugin.Program
	serverStopped  bool
	debugger       *debugSession
	debuggerConfig plugin.DebugAdapter
}

func (a *App) registerCoreModes() error {
	modes := []plugin.Mode{
		{Name: "Plain Text", Syntax: "plain"},
		{Name: "C/C++", Extensions: []string{".c", ".h", ".cc", ".cpp", ".cxx", ".hpp"}, Syntax: "c"},
		{Name: "Diff", Extensions: []string{".diff", ".patch"}, Syntax: "diff"},
		{Name: "JavaScript", Extensions: []string{".js", ".jsx", ".ts", ".tsx"}, Syntax: "javascript"},
		{Name: "Python", Extensions: []string{".py"}, Syntax: "python"},
		{Name: "Rust", Extensions: []string{".rs"}, Syntax: "rust"},
		{Name: "Shell", Extensions: []string{".sh", ".bash", ".zsh"}, Syntax: "shell"},
	}
	for _, mode := range modes {
		if err := a.RegisterMode(mode); err != nil {
			return err
		}
		a.modes[modeKey(mode.Name)].builtin = true
	}
	return nil
}

// RegisterMode adds a plugin mode, replacing built-in syntax-only fallbacks.
func (a *App) RegisterMode(mode plugin.Mode) error {
	mode.Name = strings.TrimSpace(mode.Name)
	if mode.Name == "" {
		return fmt.Errorf("mode name cannot be empty")
	}
	key := modeKey(mode.Name)
	previous := a.modes[key]
	if previous != nil && (!previous.builtin || previous.languageServer != nil || previous.debugger != nil) {
		return fmt.Errorf("mode %q is already registered", mode.Name)
	}
	if mode.Syntax == "" {
		mode.Syntax = "plain"
	}
	mode.LanguageServer.Command = strings.TrimSpace(mode.LanguageServer.Command)
	mode.LanguageServer.Arguments = append([]string(nil), mode.LanguageServer.Arguments...)
	mode.DebugAdapter.Program.Command = strings.TrimSpace(mode.DebugAdapter.Program.Command)
	mode.DebugAdapter.Program.Arguments = append([]string(nil), mode.DebugAdapter.Program.Arguments...)
	if mode.LanguageServer.Command != "" && strings.TrimSpace(mode.LanguageID) == "" {
		return fmt.Errorf("mode %s must set a language ID for its language server", mode.Name)
	}
	if mode.DebugAdapter.Transport == "" {
		mode.DebugAdapter.Transport = plugin.DebugStandardIO
	}
	if mode.DebugAdapter.Transport != plugin.DebugStandardIO &&
		mode.DebugAdapter.Transport != plugin.DebugReverseTCP {
		return fmt.Errorf("mode %s has unsupported debug transport %q", mode.Name, mode.DebugAdapter.Transport)
	}
	if mode.DebugAdapter.Program.Command != "" && mode.DebugAdapter.Transport == plugin.DebugReverseTCP &&
		!containsAddressPlaceholder(mode.DebugAdapter.Program.Arguments) {
		return fmt.Errorf("mode %s reverse TCP adapter must include {address} in its arguments", mode.Name)
	}
	extensions := make([]string, 0, len(mode.Extensions))
	seen := make(map[string]bool)
	for _, extension := range mode.Extensions {
		extension = normalizeExtension(extension)
		if extension == "" {
			return fmt.Errorf("mode %s has an empty file extension", mode.Name)
		}
		if existing := a.extensionModes[extension]; existing != "" && !a.modes[existing].builtin {
			return fmt.Errorf("extension %s is already registered by mode %s", extension, a.modes[existing].Name)
		}
		if seen[extension] {
			return fmt.Errorf("mode %s registers extension %s more than once", mode.Name, extension)
		}
		seen[extension] = true
		extensions = append(extensions, extension)
	}
	if previous != nil {
		for _, extension := range previous.Extensions {
			if a.extensionModes[extension] == key {
				delete(a.extensionModes, extension)
			}
		}
	}
	for _, extension := range extensions {
		if owner := a.modes[a.extensionModes[extension]]; owner != nil && owner.builtin {
			owner.Extensions = slices.DeleteFunc(owner.Extensions, func(value string) bool { return value == extension })
		}
	}
	mode.Extensions = extensions
	a.modes[key] = &editingMode{Mode: mode}
	for _, extension := range extensions {
		a.extensionModes[extension] = key
	}
	return nil
}

func containsAddressPlaceholder(arguments []string) bool {
	for _, argument := range arguments {
		if strings.Contains(argument, "{address}") {
			return true
		}
	}
	return false
}

func normalizeExtension(extension string) string {
	extension = strings.TrimSpace(strings.ToLower(extension))
	if extension != "" && !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	return extension
}

func modeKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (a *App) modeForPath(path string) string {
	if key := a.extensionModes[normalizeExtension(filepath.Ext(path))]; key != "" {
		return key
	}
	return modeKey("Plain Text")
}

func (a *App) modeForBuffer(current *buffer.Buffer) plugin.Mode {
	editorBuffer := a.editorBufferFor(current)
	if editorBuffer == nil {
		if plain := a.modes[modeKey("Plain Text")]; plain != nil {
			return plain.Mode
		}
		return plugin.Mode{}
	}
	key := editorBuffer.mode
	if mode, ok := a.modes[key]; ok {
		return mode.Mode
	}
	if plain := a.modes[modeKey("Plain Text")]; plain != nil {
		return plain.Mode
	}
	return plugin.Mode{}
}

func (a *App) highlighterForBuffer(current *buffer.Buffer) *syntax.Highlighter {
	editorBuffer := a.editorBufferFor(current)
	if editorBuffer == nil {
		return syntax.NewLanguage("plain")
	}
	return a.highlighterForMode(editorBuffer.mode)
}

func (a *App) highlighterForMode(mode string) *syntax.Highlighter {
	return syntax.NewLanguage(a.modes[mode].Syntax)
}

func (a *App) switchMode(arguments string) error {
	if name := strings.TrimSpace(arguments); name != "" {
		return a.setCurrentMode(name)
	}
	keys := make([]string, 0, len(a.modes))
	for key := range a.modes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return a.modes[keys[i]].Name < a.modes[keys[j]].Name
	})
	items := make([]paletteItem, 0, len(keys))
	for _, key := range keys {
		mode := a.modes[key]
		detail := strings.Join(mode.Extensions, ", ")
		items = append(items, paletteItem{label: mode.Name, detail: detail, value: key})
	}
	a.choose("Switch Mode", items, func(item paletteItem) {
		if err := a.setCurrentMode(item.value); err != nil {
			a.message = err.Error()
		}
	})
	return nil
}

func (a *App) setCurrentMode(name string) error {
	key := modeKey(name)
	mode, ok := a.modes[key]
	if !ok {
		return fmt.Errorf("unknown mode %q", name)
	}
	a.completionEpoch++
	if a.palette != nil && a.palette.completion {
		a.palette = nil
	}
	current := a.current()
	editorBuffer := a.currentEditorBuffer()
	if editorBuffer.lsp != nil {
		a.notifyLSPDidClose(current)
	}
	editorBuffer.diagnostics = nil
	editorBuffer.mode = key
	editorBuffer.highlighter = syntax.NewLanguage(mode.Syntax)
	a.activateCurrentMode()
	a.notifyLSPDidOpen(current)
	a.message = "mode: " + mode.Name
	return nil
}

func (a *App) activateCurrentMode() {
	if len(a.buffers) == 0 {
		return
	}
	mode := a.modes[a.currentEditorBuffer().mode]
	if mode == nil || mode.serverStopped {
		return
	}
	if mode.LanguageServer.Command == "" && mode.serverProgram.Command == "" {
		return
	}
	if err := a.StartLanguageServer(mode.Name); err != nil {
		a.message = fmt.Sprintf("%s language server: %v", mode.Name, err)
	}
}

func (a *App) hasLanguageServerForCurrentMode() bool {
	return a.languageServerForBuffer(a.current()) != nil
}
