package editor

import (
	"strings"
	"testing"

	"github.com/bluescreen10/myde/settings"
	"github.com/bluescreen10/myde/terminal"
)

func TestBuiltinThemesLoadFromJSON(t *testing.T) {
	loaded, ids, err := loadBuiltinThemes()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 5 || len(ids) != 5 {
		t.Fatalf("loaded %d themes with %d IDs, want 5", len(loaded), len(ids))
	}
	current, ok := loaded[defaultThemeID]
	if !ok {
		t.Fatalf("default theme %q is missing", defaultThemeID)
	}
	if current.Name != "VS Dark 2026" {
		t.Fatalf("default theme name = %q", current.Name)
	}
	if got := current.Borders.Corners.TopLeft; got != '╭' {
		t.Fatalf("default top-left corner = %q, want %q", got, '╭')
	}
	if got := current.Borders.Separator.Horizontal; got != '─' {
		t.Fatalf("default horizontal separator = %q, want %q", got, '─')
	}
	if got := current.Borders.StatusSeparator; got != '' {
		t.Fatalf("default status separator = %q, want %q", got, '')
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
	for _, id := range []string{"retro-green", "retro-orange"} {
		_, ok := loaded[id]
		if !ok {
			t.Fatalf("retro theme %q is missing", id)
		}
	}
}

func TestBorderCharactersResolveExactly(t *testing.T) {
	characters, err := resolveBorderCharacters(borderDocument{
		StatusSeparator: "x",
		Separator:       axisDocument{Horizontal: "s", Vertical: "S"},
		Corners: cornerDocument{
			TopLeft: "1", TopRight: "2", BottomLeft: "3", BottomRight: "4",
		},
		Lines: axisDocument{Horizontal: "h", Vertical: "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if characters.StatusSeparator != 'x' ||
		characters.Separator.Horizontal != 's' || characters.Separator.Vertical != 'S' ||
		characters.Corners.TopLeft != '1' || characters.Corners.TopRight != '2' ||
		characters.Corners.BottomLeft != '3' || characters.Corners.BottomRight != '4' ||
		characters.Lines.Horizontal != 'h' || characters.Lines.Vertical != 'v' {
		t.Fatalf("characters = %+v", characters)
	}
}

func TestOptionalTabColorsFallBackForExistingThemes(t *testing.T) {
	loaded, _, err := loadBuiltinThemes()
	if err != nil {
		t.Fatal(err)
	}
	current := loaded["midnight"]
	checks := []struct {
		name string
		got  terminal.Color
		want terminal.Color
	}{
		{name: "selection text", got: current.SelectionText, want: current.StatusText},
		{name: "tab background", got: current.TabBackground, want: current.Background},
		{name: "tab text", got: current.TabText, want: current.Muted},
		{name: "active tab background", got: current.TabActiveBackground, want: current.Selection},
		{name: "active tab text", got: current.TabActiveText, want: current.Accent},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("fallback %s = %+v, want %+v", check.name, check.got, check.want)
		}
	}
}

func TestThemeRejectsInvalidBorderCharacter(t *testing.T) {
	_, err := resolveBorderCharacters(borderDocument{
		Separator: axisDocument{Horizontal: "two", Vertical: "S"},
		Corners: cornerDocument{
			TopLeft: "1", TopRight: "2", BottomLeft: "3", BottomRight: "4",
		},
		Lines: axisDocument{Horizontal: "h", Vertical: "v"},
	})
	if err == nil || !strings.Contains(err.Error(), "must be one character") {
		t.Fatalf("error = %v", err)
	}
}

func TestThemeSelectorListsAndActivatesLoadedThemes(t *testing.T) {
	loaded, ids, err := loadBuiltinThemes()
	if err != nil {
		t.Fatal(err)
	}
	app := &App{theme: loaded[defaultThemeID], themes: loaded, themeIDs: ids}
	if err := app.selectTheme(""); err != nil {
		t.Fatal(err)
	}
	if app.palette == nil {
		t.Fatal("theme selector did not open")
	}
	if len(app.palette.items) != len(loaded) {
		t.Fatalf("selector has %d items, want %d", len(app.palette.items), len(loaded))
	}
	for _, item := range app.palette.items {
		if item.value != "midnight" {
			continue
		}
		app.palette.onChoose(item)
		if app.theme.ID != "midnight" {
			t.Fatalf("active theme = %q", app.theme.ID)
		}
		return
	}
	t.Fatal("midnight theme was not listed")
}

func TestThemeSelectionPersistsUserSetting(t *testing.T) {
	loaded, _, err := loadBuiltinThemes()
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{theme: loaded[defaultThemeID], themes: loaded, settings: store}
	if err := app.activateTheme("midnight"); err != nil {
		t.Fatal(err)
	}
	reopened, err := settings.OpenAt(store.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Value("theme"); got != "midnight" {
		t.Fatalf("saved theme = %q, want midnight", got)
	}
}
