package editor_test

import (
	"github.com/bluescreen10/myde/plugin"
	"strings"
	"testing"
	"time"
)

func TestRegisterStatusRefreshesInBackground(t *testing.T) {
	h := newHarness(t, "")
	release := make(chan struct{})
	defer close(release)
	if err := h.app.RegisterStatus("example.branch", plugin.StatusItem{Text: "old", RefreshInterval: time.Hour, OnRefresh: func() (string, error) { <-release; return "  ⎇ main  ", nil }}); err != nil {
		t.Fatal(err)
	}
	if err := h.app.RegisterStatus("example.branch", plugin.StatusItem{}); err == nil {
		t.Fatal("duplicate status accepted")
	}
	h.app.Update()
	h.text("a")
	if h.content() != "a" {
		t.Fatal("slow status blocked input")
	}
	release <- struct{}{}
	h.wait("status", func() bool { return strings.Contains(h.screen.row(23), "⎇ main") })
}
func TestEditorStatusShowsModePositionAndPercentage(t *testing.T) {
	h := newHarness(t, "")
	h.replace("one\ntwo\nthree")
	h.exec("cursor.goto-line 2")
	row := h.screen.row(23)
	for _, want := range []string{"Plain Text", "2:1", "66%"} {
		if !strings.Contains(row, want) {
			t.Fatalf("status %q missing %q", row, want)
		}
	}
}
func TestScrollPercentage(t *testing.T) {
	h := newHarness(t, "")
	h.replace("1\n2\n3\n4")
	for _, test := range []struct {
		line int
		want string
	}{{1, "25%"}, {2, "50%"}, {4, "100%"}} {
		h.exec("cursor.goto-line " + string(rune('0'+test.line)))
		if !strings.Contains(h.screen.row(23), test.want) {
			t.Fatalf("status=%q", h.screen.row(23))
		}
	}
}
func TestStatusMessageExpires(t *testing.T) {
	h := newHarness(t, "")
	h.app.SetMessage("transient status")
	h.draw()
	if !h.contains("transient status") {
		t.Fatal("message missing")
	}
	h.wait("message expiration", func() bool { return !h.contains("transient status") })
}
