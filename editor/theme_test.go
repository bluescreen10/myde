package editor_test

import (
	"github.com/bluescreen10/myde/editor"
	"github.com/bluescreen10/myde/settings"
	"github.com/bluescreen10/myde/terminal"
	"strings"
	"testing"
)

func TestBuiltinDefaultThemeColors(t *testing.T) {
	current := editor.VSDark2026()
	if current.Name != "VS Dark 2026" || current.Borders.Corners.TopLeft != '╭' || current.Borders.Separator.Horizontal != '─' {
		t.Fatalf("theme=%+v", current)
	}
	colors := []struct {
		name string
		got  terminal.Color
		want string
	}{
		{name: "background", got: current.Background, want: "#121314"},
		{name: "foreground", got: current.Foreground, want: "#BBBEBF"},
		{name: "muted", got: current.Muted, want: "#8C8C8C"},
		{name: "selection", got: current.Selection, want: "#4FA8FF"},
		{name: "selection text", got: current.SelectionText, want: "#000000"},
		{name: "tab background", got: current.TabBackground, want: "#121314"},
		{name: "tab text", got: current.TabText, want: "#8C8C8C"},
		{name: "active tab background", got: current.TabActiveBackground, want: "#4FA8FF"},
		{name: "active tab text", got: current.TabActiveText, want: "#000000"},
		{name: "cursor", got: current.Cursor, want: "#BBBEBF"},
		{name: "status", got: current.Status, want: "#191A1B"},
		{name: "status text", got: current.StatusText, want: "#8C8C8C"},
		{name: "accent", got: current.Accent, want: "#4FA8FF"},
		{name: "success", got: current.Success, want: "#73C991"},
		{name: "warning", got: current.Warning, want: "#E5BA7D"},
		{name: "danger", got: current.Danger, want: "#F48771"},
		{name: "error", got: current.Error, want: "#F48771"},
		{name: "diagnostic", got: current.Diagnostic, want: "#F48771"},
		{name: "panel", got: current.Panel, want: "#191A1B"},
		{name: "panel border", got: current.PanelBorder, want: "#2A2B2C"},
		{name: "diagnostic background", got: current.DiagnosticBackground, want: "#3A1D1D"},
		{name: "plain", got: current.Plain, want: "#C9D1D9"},
		{name: "comment", got: current.Comment, want: "#8B949E"},
		{name: "keyword", got: current.Keyword, want: "#C586C0"},
		{name: "string", got: current.String, want: "#A5D6FF"},
		{name: "number", got: current.Number, want: "#79C0FF"},
		{name: "type", got: current.Type, want: "#4EC9B0"},
		{name: "import", got: current.Import, want: "#FFA657"},
		{name: "declaration", got: current.Declaration, want: "#FF7B72"},
		{name: "function", got: current.Function, want: "#D2A8FF"},
		{name: "delimiter", got: current.Delimiter, want: "#FFD700"},
		{name: "delimiter 2", got: current.Delimiter2, want: "#DA70D6"},
		{name: "delimiter 3", got: current.Delimiter3, want: "#179FFF"},
		{name: "constant", got: current.Constant, want: "#569CD6"},
	}
	for _, color := range colors {
		want, err := terminal.ParseColor(color.want)
		if err != nil {
			t.Fatal(err)
		}
		if color.got != want {
			t.Errorf("default %s = %+v, want %s", color.name, color.got, color.want)
		}
	}
}

func TestThemeSelectorListsAndActivatesLoadedThemes(t *testing.T) {
	h := newHarness(t, "")
	h.exec("theme.select")
	for _, want := range []string{"Midnight", "VS Dark 2026", "Retro"} {
		if !h.contains(want) {
			t.Fatalf("theme selector missing %q:\n%s", want, h.screen.text())
		}
	}
	h.key(terminal.Event{Key: terminal.KeyEscape})
	h.exec("theme.select midnight")
	if err := h.app.Execute("theme.select unknown-theme"); err == nil {
		t.Fatal("unknown theme accepted")
	}
}

func TestThemeSelectionPersistsUserSetting(t *testing.T) {
	store, err := settings.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := harnessWithSettings(t, "", store)
	h.exec("theme.select midnight")
	reopened, err := settings.OpenAt(store.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Value("theme") != "midnight" {
		t.Fatal("theme setting did not persist")
	}
	other := harnessWithSettings(t, "", reopened)
	other.exec("theme.select")
	if !strings.Contains(other.screen.text(), "Midnight") {
		t.Fatal("saved theme could not reload")
	}
}
