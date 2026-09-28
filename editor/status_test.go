package editor

import (
	"reflect"
	"testing"
	"time"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
)

func TestRegisterStatusRefreshesInBackground(t *testing.T) {
	app := &App{
		statusNames: make(map[string]bool),
		servers:     make(chan serverEvent, 1),
	}
	if err := app.RegisterStatus("example.branch", plugin.StatusItem{
		Text:            "⎇ old",
		RefreshInterval: time.Minute,
		OnRefresh: func() (string, error) {
			return "  ⎇ main  ", nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.RegisterStatus("example.branch", plugin.StatusItem{Text: "duplicate"}); err == nil {
		t.Fatal("duplicate status item was accepted")
	}

	app.pollStatusItems()
	select {
	case event := <-app.servers:
		app.handleServerEvent(event)
	case <-time.After(time.Second):
		t.Fatal("status refresh did not finish")
	}
	if got := app.statusItems[0].text; got != "⎇ main" {
		t.Fatalf("status text = %q, want %q", got, "⎇ main")
	}
}

func TestEditorStatusSectionsOrderAndDiagnostics(t *testing.T) {
	current := buffer.New()
	current.Insert(0, []byte("one\ntwo\nthree"))
	app := &App{
		buffers: []*editorBuffer{{
			text: current,
			mode: modeKey("Go"),
			diagnostics: []diagnostic{
				{severity: 1},
				{severity: 1},
				{severity: 2},
				{severity: 3},
			},
		}},
		modes: map[string]plugin.Mode{
			modeKey("Go"): {Name: "Go"},
		},
		theme:       VSDark2026(),
		statusItems: []*statusItem{{text: "⎇ main"}},
	}
	sections := app.editorStatusSections(buffer.Point{Line: 1, Column: 2})
	texts := make([]string, len(sections))
	for index, section := range sections {
		texts[index] = section.text
	}
	want := []string{"⎇ main", "2✖ 1⚠", "Go", "2:3", "66%"}
	if !reflect.DeepEqual(texts, want) {
		t.Fatalf("status sections = %#v, want %#v", texts, want)
	}
}

func TestScrollPercentage(t *testing.T) {
	tests := []struct {
		line      int
		lineCount int
		want      int
	}{
		{line: 0, lineCount: 1, want: 100},
		{line: 0, lineCount: 4, want: 25},
		{line: 1, lineCount: 4, want: 50},
		{line: 3, lineCount: 4, want: 100},
	}
	for _, test := range tests {
		if got := scrollPercentage(test.line, test.lineCount); got != test.want {
			t.Errorf("scrollPercentage(%d, %d) = %d, want %d", test.line, test.lineCount, got, test.want)
		}
	}
}

func TestStatusMessageExpires(t *testing.T) {
	app := &App{message: "saved main.go"}
	shownAt := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	app.updateMessageLifetime(shownAt)
	if app.message != "saved main.go" {
		t.Fatalf("message cleared before its deadline: %q", app.message)
	}
	app.updateMessageLifetime(shownAt.Add(messageDisplayDuration - time.Millisecond))
	if app.message != "saved main.go" {
		t.Fatalf("message cleared before its deadline: %q", app.message)
	}
	app.updateMessageLifetime(shownAt.Add(messageDisplayDuration))
	if app.message != "" {
		t.Fatalf("expired message = %q, want empty", app.message)
	}
}
