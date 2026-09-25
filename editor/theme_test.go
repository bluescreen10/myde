package editor

import (
	"strings"
	"testing"
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
	for _, id := range []string{"retro-green", "retro-orange"} {
		_, ok := loaded[id]
		if !ok {
			t.Fatalf("retro theme %q is missing", id)
		}
	}
}

func TestBorderCharactersResolveExactly(t *testing.T) {
	characters, err := resolveBorderCharacters(borderDocument{
		Separator: axisDocument{Horizontal: "s", Vertical: "S"},
		Corners: cornerDocument{
			TopLeft: "1", TopRight: "2", BottomLeft: "3", BottomRight: "4",
		},
		Lines: axisDocument{Horizontal: "h", Vertical: "v"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if characters.Separator.Horizontal != 's' || characters.Separator.Vertical != 'S' ||
		characters.Corners.TopLeft != '1' || characters.Corners.TopRight != '2' ||
		characters.Corners.BottomLeft != '3' || characters.Corners.BottomRight != '4' ||
		characters.Lines.Horizontal != 'h' || characters.Lines.Vertical != 'v' {
		t.Fatalf("characters = %+v", characters)
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
