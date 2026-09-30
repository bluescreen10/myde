package editor

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/syntax"
	"github.com/bluescreen10/myde/terminal"
)

func TestSubscribeValidatesNamesAndPreservesOrder(t *testing.T) {
	app := &App{}
	for _, name := range []string{"file", ".open", "file.", "File.open", "file.before_save", "file.save.again"} {
		if err := app.Subscribe(name, func(plugin.Event) error { return nil }); err == nil {
			t.Fatalf("Subscribe(%q) accepted an invalid event name", name)
		}
	}
	if err := app.Subscribe("file.open", nil); err == nil {
		t.Fatal("Subscribe accepted a nil handler")
	}
	var order []int
	for index := range 2 {
		index := index
		if err := app.Subscribe("file.open", func(plugin.Event) error {
			order = append(order, index)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.publishEvent("file.open", "/tmp/a.go", nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []int{0, 1}) {
		t.Fatalf("subscriber order = %v", order)
	}
}

func TestFileLifecycleEventsWrapSaveAndClose(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plain := modeKey("Plain Text")
	app := &App{
		root:           root,
		screen:         terminal.NewScreen(io.Discard, 80, 24),
		modes:          map[string]plugin.Mode{plain: {Name: "Plain Text", Syntax: "plain"}},
		extensionModes: map[string]string{".go": plain},
		extensions:     &extensions{},
	}
	var events []plugin.Event
	for _, name := range []string{
		plugin.EventFileOpen,
		plugin.EventFileBeforeSave,
		plugin.EventFileAfterSave,
		plugin.EventFileClose,
	} {
		if err := app.Subscribe(name, func(event plugin.Event) error {
			events = append(events, event)
			if event.Name == plugin.EventFileBeforeSave {
				return app.ReplaceCurrentDocument([]byte("formatted\n"))
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.open(path); err != nil {
		t.Fatal(err)
	}
	app.current().Insert(app.current().Len(), []byte("dirty\n"))
	if err := app.save(""); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "formatted\n" {
		t.Fatalf("saved contents = %q", contents)
	}
	app.closeBufferNow(app.current())
	want := []string{
		plugin.EventFileOpen,
		plugin.EventFileBeforeSave,
		plugin.EventFileAfterSave,
		plugin.EventFileClose,
	}
	if len(events) != len(want) {
		t.Fatalf("events = %+v", events)
	}
	for index, event := range events {
		if event.Name != want[index] || event.Value != path {
			t.Fatalf("event %d = %+v, want %s with %s", index, event, want[index], path)
		}
	}
}

func TestBeforeSaveErrorCancelsWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	if err := os.WriteFile(path, []byte("on disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	current, err := buffer.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	current.Insert(current.Len(), []byte("edited\n"))
	app := &App{
		root: root,
		buffers: []*editorBuffer{{
			text: current, highlighter: syntax.NewLanguage("plain"), mode: modeKey("Plain Text"),
		}},
		modes:      map[string]plugin.Mode{modeKey("Plain Text"): {Name: "Plain Text", Syntax: "plain"}},
		extensions: &extensions{},
	}
	if err := app.Subscribe(plugin.EventFileBeforeSave, func(plugin.Event) error {
		return errors.New("formatter failed")
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.save(""); err == nil || err.Error() != "file.before-save: formatter failed" {
		t.Fatalf("save error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "on disk\n" {
		t.Fatalf("failed before-save changed disk contents to %q", contents)
	}
}

func TestBeforeSaveTargetsInactiveBuffer(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "one.go"), filepath.Join(root, "two.go")}
	app := &App{
		root:       root,
		modes:      map[string]plugin.Mode{modeKey("Plain Text"): {Name: "Plain Text", Syntax: "plain"}},
		extensions: &extensions{},
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		current, err := buffer.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		current.Insert(current.Len(), []byte("dirty\n"))
		app.buffers = append(app.buffers, &editorBuffer{
			text: current, highlighter: syntax.NewLanguage("plain"), mode: modeKey("Plain Text"),
		})
	}
	if err := app.Subscribe(plugin.EventFileBeforeSave, func(plugin.Event) error {
		return app.ReplaceCurrentDocument([]byte(filepath.Base(app.CurrentPath()) + "\n"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.saveBufferForQuit(app.buffers[1].text, ""); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(paths[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "two.go\n" || app.active != 0 {
		t.Fatalf("inactive save contents = %q, active = %d", contents, app.active)
	}
}
