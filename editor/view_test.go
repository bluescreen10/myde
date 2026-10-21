package editor_test

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

func reviewView(choose func(ui.Action, ui.ListItem) error) ui.View {
	return ui.View{Title: "Review", Layout: ui.Layout{Panes: []ui.Pane{
		{ID: "files", List: &ui.List{Sections: []ui.ListSection{{Title: "Files", Items: []ui.ListItem{{Label: "a.go", Value: "a.go"}, {Label: "b.go", Value: "b.go"}}}}, PreviewPane: "preview", OnAction: choose, OnSelect: func(item ui.ListItem) (ui.Widget, error) {
			return ui.TextWidget{Title: "preview-" + item.Value, Content: []byte(item.Value)}, nil
		}}},
		{ID: "preview"},
	}}}
}

func TestViewListLoadsPreviewAndActivatesSelection(t *testing.T) {
	h := newHarness(t, "")
	chosen := ""
	h.app.NewView(reviewView(func(_ ui.Action, item ui.ListItem) error { chosen = item.Value; return nil }))
	h.wait("first preview", func() bool { return strings.Contains(h.screen.row(1), "preview-a.go") })
	h.key(terminal.Event{Key: terminal.KeyDown})
	h.wait("second preview", func() bool { return strings.Contains(h.screen.row(1), "preview-b.go") })
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if chosen != "b.go" {
		t.Fatalf("activated=%q", chosen)
	}
	chosen = ""
	h.key(terminal.Event{Key: terminal.KeyTab})
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if chosen != "b.go" {
		t.Fatalf("preview activated=%q", chosen)
	}
}

func TestViewUpdateHonorsExplicitSelectionAndPreservesImplicitSelection(t *testing.T) {
	h := newHarness(t, "")
	chosen := ""
	def := reviewView(func(_ ui.Action, item ui.ListItem) error { chosen = item.Value; return nil })
	handle := h.app.NewView(def)
	h.key(terminal.Event{Key: terminal.KeyDown})
	if !handle.Update(def) {
		t.Fatal("update failed")
	}
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if chosen != "b.go" {
		t.Fatalf("lost selection=%q", chosen)
	}
	def.Layout.Panes[0].List.SelectedValue = "a.go"
	handle.Update(def)
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if chosen != "a.go" {
		t.Fatalf("explicit selection=%q", chosen)
	}
}

func TestViewHandleUsesBufferTabLifecycle(t *testing.T) {
	h := newHarness(t, "")
	handle := h.app.NewView(ui.View{Title: "Review", Layout: ui.Layout{Panes: []ui.Pane{{ID: "one"}}}})
	h.draw()
	if !h.contains("Review") || !handle.Show() {
		t.Fatal("view not raised")
	}
	handle.Destroy()
	if handle.Show() || handle.Update(ui.View{}) {
		t.Fatal("destroyed view is live")
	}
	if h.app.CurrentDocument().ReadOnly {
		t.Fatal("did not return to editable buffer")
	}
}

func TestViewIgnoresResponsesFromReplacedPanes(t *testing.T) {
	h := newHarness(t, "")
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	def := reviewView(nil)
	def.Layout.Panes[0].List.OnSelect = func(ui.ListItem) (ui.Widget, error) {
		started <- struct{}{}
		<-release
		return ui.TextWidget{Content: []byte("STALE_PREVIEW")}, nil
	}
	handle := h.app.NewView(def)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("preview did not start")
	}
	replacement := ui.View{Title: "New", Layout: ui.Layout{Panes: []ui.Pane{{ID: "preview", Widget: ui.TextWidget{Content: []byte("CURRENT_PREVIEW")}}}}}
	handle.Update(replacement)
	close(release)
	h.wait("replacement preview", func() bool { return h.contains("CURRENT_PREVIEW") })
	// Exercise more updates after releasing the old provider.
	for range 20 {
		h.app.Update()
		h.draw()
		time.Sleep(time.Millisecond)
	}
	if h.contains("STALE_PREVIEW") {
		t.Fatal("stale preview replaced current content")
	}
}

func TestViewClearsPreviewWhenListBecomesEmpty(t *testing.T) {
	h := newHarness(t, "")
	def := reviewView(nil)
	handle := h.app.NewView(def)
	h.wait("preview", func() bool { return h.contains("preview-a.go") })
	def.Layout.Panes[0].List.Sections = nil
	handle.Update(def)
	h.draw()
	if strings.Contains(h.screen.row(1), "preview-a.go") {
		t.Fatal("empty list kept stale preview")
	}
}

func TestViewBackgroundRefreshPreservesSelection(t *testing.T) {
	h := newHarness(t, "")
	chosen := ""
	var refreshes atomic.Int32
	def := reviewView(func(_ ui.Action, item ui.ListItem) error { chosen = item.Value; return nil })
	updated := def
	def.RefreshInterval = time.Millisecond
	def.OnRefresh = func() (ui.View, error) { refreshes.Add(1); return updated, nil }
	h.app.NewView(def)
	h.key(terminal.Event{Key: terminal.KeyDown})
	h.wait("background refresh", func() bool { return refreshes.Load() > 0 })
	h.key(terminal.Event{Key: terminal.KeyEnter})
	if chosen != "b.go" {
		t.Fatalf("refresh selection=%q", chosen)
	}
}

func TestRichTextWidgetPlacementAndSemanticStyle(t *testing.T) {
	h := newHarness(t, "")
	h.app.NewView(ui.View{Title: "Rich", Layout: ui.Layout{Panes: []ui.Pane{{ID: "rich", Widget: ui.WidgetContent{Lines: []ui.RichTextLine{{Spans: []ui.RichTextSpan{{Text: "ab"}, {Text: "x", Positioned: true, Column: 8}, {Text: "\tZ", Style: ui.WidgetStyle{Foreground: ui.ToneAccent}}}}}}}}}})
	h.draw()
	if !strings.Contains(h.screen.row(2), "ab      x   Z") {
		t.Fatalf("widget row=%q", h.screen.row(2))
	}
}
