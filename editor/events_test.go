package editor_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/terminal"
)

func TestSubscribeValidatesNamesAndPreservesOrder(t *testing.T) {
	h := newHarness(t, "")
	for _, name := range []string{"file", ".open", "file.", "File.open", "file.before_save", "file.save.again"} {
		if err := h.app.Subscribe(name, func(plugin.Event) error { return nil }); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	if err := h.app.Subscribe("file.open", nil); err == nil {
		t.Fatal("accepted nil handler")
	}
	var order []int
	for i := range 2 {
		if err := h.app.Subscribe("file.open", func(plugin.Event) error { order = append(order, i); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	h.exec("file.new a.txt")
	if !reflect.DeepEqual(order, []int{0, 1}) {
		t.Fatalf("order=%v", order)
	}
}

func TestFileLifecycleEventsWrapSaveAndClose(t *testing.T) {
	h := newHarness(t, "")
	var events []plugin.Event
	for _, name := range []string{plugin.EventFileOpen, plugin.EventFileBeforeSave, plugin.EventFileAfterSave, plugin.EventFileClose} {
		if err := h.app.Subscribe(name, func(event plugin.Event) error {
			events = append(events, event)
			if event.Name == plugin.EventFileBeforeSave {
				return h.app.ReplaceCurrentDocument([]byte("formatted\n"))
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	h.exec("file.new main.go")
	h.text("dirty")
	h.exec("file.save")
	h.exec("buffer.close")
	want := []string{plugin.EventFileOpen, plugin.EventFileBeforeSave, plugin.EventFileAfterSave, plugin.EventFileClose}
	if len(events) != len(want) {
		t.Fatalf("events=%+v", events)
	}
	for i, event := range events {
		if event.Name != want[i] || event.Value != filepath.Join(h.root, "main.go") {
			t.Fatalf("event=%+v", event)
		}
	}
	data, err := os.ReadFile(filepath.Join(h.root, "main.go"))
	if err != nil || string(data) != "formatted\n" {
		t.Fatalf("saved=%q, %v", data, err)
	}
}

func TestBeforeSaveErrorCancelsWrite(t *testing.T) {
	h := newHarness(t, "")
	path := filepath.Join(h.root, "file.txt")
	if err := os.WriteFile(path, []byte("on disk\n"), 0600); err != nil {
		t.Fatal(err)
	}
	h.exec("file.open file.txt")
	h.text("edited")
	if err := h.app.Subscribe(plugin.EventFileBeforeSave, func(plugin.Event) error { return errors.New("formatter failed") }); err != nil {
		t.Fatal(err)
	}
	if err := h.app.Execute("file.save"); err == nil {
		t.Fatal("save accepted rejected before-save event")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "on disk\n" {
		t.Fatalf("saved=%q, %v", data, err)
	}
}

func TestBeforeSaveTargetsInactiveBuffer(t *testing.T) {
	h := newHarness(t, "")
	h.exec("file.new one.go")
	h.text("one")
	h.exec("file.new two.go")
	h.text("two")
	if err := h.app.Subscribe(plugin.EventFileBeforeSave, func(plugin.Event) error {
		return h.app.ReplaceCurrentDocument([]byte(filepath.Base(h.app.CurrentPath()) + "\n"))
	}); err != nil {
		t.Fatal(err)
	}
	h.exec("editor.quit")
	h.key(terminal.Event{Key: terminal.KeyEnter})
	for _, name := range []string{"one.go", "two.go"} {
		data, err := os.ReadFile(filepath.Join(h.root, name))
		if err != nil || string(data) != name+"\n" {
			t.Fatalf("%s=%q, %v", name, data, err)
		}
	}
}
