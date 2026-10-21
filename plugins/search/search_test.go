package search_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/plugins/search"
	"github.com/bluescreen10/myde/ui"
)

// The plugin uses only these host services. Embedding the interface keeps the
// test independent of unrelated language, debugger, and view services.
type host struct {
	plugin.Host
	root      string
	commands  map[string]plugin.Command
	bindings  map[string]string
	view      *ui.View
	created   int
	handle    *viewHandle
	documents []plugin.Document
	opened    string
	position  plugin.Position
}

func (h *host) Root() string { return h.root }
func (h *host) RegisterGlobalCommand(name string, command plugin.Command) error {
	h.commands[name] = command
	return nil
}
func (h *host) RegisterBinding(key, command string) error {
	h.bindings[key] = command
	return nil
}

type viewHandle struct {
	host  *host
	alive bool
}

func (h *viewHandle) Show() bool { return h.alive }
func (h *viewHandle) Update(view ui.View) bool {
	if !h.alive {
		return false
	}
	h.host.view = &view
	return true
}
func (h *viewHandle) Destroy() { h.alive = false }
func (h *host) NewView(view ui.View) ui.ViewHandle {
	h.created++
	h.view = &view
	h.handle = &viewHandle{host: h, alive: true}
	return h.handle
}
func (h *host) OpenDocuments() []plugin.Document { return h.documents }
func (h *host) OpenLocation(path string, position plugin.Position) error {
	h.opened, h.position = path, position
	return nil
}

func load(t *testing.T) *host {
	t.Helper()
	h := &host{root: t.TempDir(), commands: make(map[string]plugin.Command), bindings: make(map[string]string)}
	p := search.New()
	if p.Name() != "search" {
		t.Fatalf("plugin name = %q", p.Name())
	}
	if err := p.Load(h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.(io.Closer).Close(); err != nil {
			t.Error(err)
		}
	})
	if h.bindings["ctrl-shift-f"] != "search.project" || h.commands["search.project"] == nil {
		t.Fatal("plugin did not register its global command and shortcut")
	}
	return h
}

func (h *host) open(t *testing.T, query string) {
	t.Helper()
	if err := h.commands["search.project"](query); err != nil {
		t.Fatal(err)
	}
}
func (h *host) refresh(t *testing.T) {
	t.Helper()
	h.view.BeforeRefresh()
	view, err := h.view.OnRefresh()
	if err != nil {
		t.Fatal(err)
	}
	h.view = &view
}
func (h *host) results() *ui.List { return h.view.Layout.Panes[0].List }

func (h *host) write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.root, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPluginRetainsSearchAndRescansReopenedFiles(t *testing.T) {
	h := load(t)
	h.write(t, "source.txt", "first needle\nλ needle\n")
	h.open(t, "needle")
	h.refresh(t)
	if len(h.results().Sections) != 1 || len(h.results().Sections[0].Items) != 2 {
		t.Fatalf("sections = %+v", h.results().Sections)
	}
	selected := h.results().Sections[0].Items[1]
	widget, err := h.results().OnSelect(selected)
	if err != nil {
		t.Fatal(err)
	}
	preview := widget.RenderWidget()
	if preview.Title != "source.txt:2" || preview.FocusLine != 1 || preview.FocusColumn != 2 || len(preview.Lines) != 2 || preview.Lines[1].Spans[1].Text != "needle" || preview.Lines[1].Spans[1].Style.Background != ui.ToneAccent {
		t.Fatalf("preview did not identify and highlight the selected location: %+v", preview)
	}
	h.open(t, "")
	if h.results().Input.Value != "needle" || len(h.results().Sections) != 1 || h.created != 1 {
		t.Fatalf("search view was not retained: %+v (created %d tabs)", h.view, h.created)
	}
	if err := h.results().OnAction(ui.Activate, selected); err != nil {
		t.Fatal(err)
	}
	if h.opened != "source.txt" || h.position != (plugin.Position{Line: 1, Column: 2}) {
		t.Fatalf("opened %q at %+v", h.opened, h.position)
	}
	h.write(t, "source.txt", "replacement needle\n")
	h.open(t, "")
	h.refresh(t)
	if len(h.results().Sections) != 1 || len(h.results().Sections[0].Items) != 1 || !strings.Contains(h.results().Sections[0].Items[0].Label, "replacement") {
		t.Fatalf("results were not rescanned: %+v", h.results().Sections)
	}
	h.handle.Destroy()
	h.open(t, "")
	if h.created != 2 || h.results().Input.Value != "needle" {
		t.Fatal("closing and reopening the tab did not restore the search")
	}
}

func TestPluginSearchesDocumentSnapshotsAndKeepsInstancesIndependent(t *testing.T) {
	h, other := load(t), load(t)
	h.write(t, "source.txt", "needle on disk\n")
	h.documents = []plugin.Document{{Path: filepath.Join(h.root, "source.txt"), Content: []byte("unsaved text\n"), Dirty: true}}
	h.open(t, "needle")
	h.refresh(t)
	if len(h.results().Sections) != 0 || h.results().Message != "No matches" {
		t.Fatalf("disk matches were not replaced by document snapshot: %+v", h.view)
	}
	view, err := h.results().OnInput("unsaved")
	if err != nil {
		t.Fatal(err)
	}
	h.view = &view
	h.refresh(t)
	if len(h.results().Sections) != 1 || !strings.Contains(h.results().Sections[0].Items[0].Label, "unsaved text") {
		t.Fatalf("unsaved match missing: %+v", h.results().Sections)
	}
	widget, err := h.results().OnSelect(h.results().Sections[0].Items[0])
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, span := range widget.RenderWidget().Lines[0].Spans {
		text.WriteString(span.Text)
	}
	if text.String() != "unsaved text" {
		t.Fatalf("preview did not use the unsaved document: %q", text.String())
	}
	h.documents = nil
	h.refresh(t)
	if len(h.results().Sections) != 0 {
		t.Fatal("results did not update after document snapshot was removed")
	}
	other.open(t, "")
	if other.results().Input.Value != "" || len(other.results().Sections) != 0 {
		t.Fatal("plugin instances shared search state")
	}
}
