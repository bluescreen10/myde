// Package editor implements the interactive terminal editor.
package editor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bluescreen10/myde/settings"
	"github.com/bluescreen10/myde/syntax"
	"github.com/bluescreen10/myde/terminal"
	themefiles "github.com/bluescreen10/myde/themes"
)

const defaultThemeID = settings.DefaultTheme

// BorderAxis contains the glyphs for horizontal and vertical strokes.
type BorderAxis struct {
	Horizontal rune
	Vertical   rune
}

// BorderCorners contains the four panel corner glyphs.
type BorderCorners struct {
	TopLeft     rune
	TopRight    rune
	BottomLeft  rune
	BottomRight rune
}

// BorderCharacters contains every glyph used to draw panels and separators.
type BorderCharacters struct {
	StatusSeparator rune
	Separator       BorderAxis
	Corners         BorderCorners
	Lines           BorderAxis
}

// Theme defines editor colors and panel decoration.
type Theme struct {
	ID   string
	Name string

	Background           terminal.Color
	Foreground           terminal.Color
	Muted                terminal.Color
	Selection            terminal.Color
	SelectionText        terminal.Color
	TabBackground        terminal.Color
	TabText              terminal.Color
	TabActiveBackground  terminal.Color
	TabActiveText        terminal.Color
	Cursor               terminal.Color
	Status               terminal.Color
	StatusText           terminal.Color
	Accent               terminal.Color
	Success              terminal.Color
	Warning              terminal.Color
	Danger               terminal.Color
	Error                terminal.Color
	Diagnostic           terminal.Color
	Panel                terminal.Color
	PanelBorder          terminal.Color
	DiagnosticBackground terminal.Color

	Comment     terminal.Color
	Plain       terminal.Color
	Keyword     terminal.Color
	String      terminal.Color
	Number      terminal.Color
	Type        terminal.Color
	Import      terminal.Color
	Declaration terminal.Color
	Function    terminal.Color
	Delimiter   terminal.Color
	Delimiter2  terminal.Color
	Delimiter3  terminal.Color
	Constant    terminal.Color

	Borders BorderCharacters
}

type themeDocument struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Colors  map[string]string `json:"colors"`
	Borders borderDocument    `json:"borders"`
}

type borderDocument struct {
	StatusSeparator string         `json:"status_separator"`
	Separator       axisDocument   `json:"separator"`
	Corners         cornerDocument `json:"corners"`
	Lines           axisDocument   `json:"lines"`
}

type axisDocument struct {
	Horizontal string `json:"horizontal"`
	Vertical   string `json:"vertical"`
}

type cornerDocument struct {
	TopLeft     string `json:"top_left"`
	TopRight    string `json:"top_right"`
	BottomLeft  string `json:"bottom_left"`
	BottomRight string `json:"bottom_right"`
}

var requiredThemeColors = []string{
	"background",
	"foreground",
	"muted",
	"selection",
	"cursor",
	"status",
	"statustext",
	"accent",
	"success",
	"warning",
	"danger",
	"error",
	"diagnostic",
	"panel",
	"panelborder",
	"diagnosticbackground",
	"comment",
	"keyword",
	"string",
	"number",
	"type",
}

// VSDark2026 returns the default theme loaded from its built-in JSON file.
func VSDark2026() Theme {
	data, err := fs.ReadFile(themefiles.Builtin, defaultThemeID+".json")
	if err != nil {
		panic(err)
	}
	loaded, err := parseTheme(data)
	if err != nil {
		panic(err)
	}
	return loaded
}

func loadBuiltinThemes() (map[string]Theme, []string, error) {
	paths, err := fs.Glob(themefiles.Builtin, "*.json")
	if err != nil {
		return nil, nil, fmt.Errorf("find built-in themes: %w", err)
	}
	sort.Strings(paths)
	loaded := make(map[string]Theme, len(paths))
	for _, path := range paths {
		data, err := fs.ReadFile(themefiles.Builtin, path)
		if err != nil {
			return nil, nil, fmt.Errorf("read built-in theme %s: %w", path, err)
		}
		current, err := parseTheme(data)
		if err != nil {
			return nil, nil, fmt.Errorf("load built-in theme %s: %w", path, err)
		}
		if _, exists := loaded[current.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate built-in theme ID %q", current.ID)
		}
		loaded[current.ID] = current
	}
	if _, exists := loaded[defaultThemeID]; !exists {
		return nil, nil, fmt.Errorf("default theme %q is not available", defaultThemeID)
	}
	ids := make([]string, 0, len(loaded))
	for id := range loaded {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return loaded[ids[i]].Name < loaded[ids[j]].Name
	})
	return loaded, ids, nil
}

func parseTheme(data []byte) (Theme, error) {
	var document themeDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Theme{}, err
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return Theme{}, err
	}
	document.ID = strings.TrimSpace(document.ID)
	document.Name = strings.TrimSpace(document.Name)
	if document.ID == "" {
		return Theme{}, fmt.Errorf("theme ID is empty")
	}
	if document.Name == "" {
		return Theme{}, fmt.Errorf("theme name is empty")
	}
	theme := Theme{ID: document.ID, Name: document.Name}
	seen := make(map[string]bool, len(document.Colors))
	for name, value := range document.Colors {
		normalized := normalizeColorName(name)
		if seen[normalized] {
			return Theme{}, fmt.Errorf("duplicate theme color %q", name)
		}
		seen[normalized] = true
		if err := theme.setColor(name, value); err != nil {
			return Theme{}, err
		}
	}
	for _, name := range requiredThemeColors {
		if !seen[name] {
			return Theme{}, fmt.Errorf("missing theme color %q", name)
		}
	}
	if !seen["selectiontext"] {
		theme.SelectionText = theme.StatusText
	}
	if !seen["tabbackground"] {
		theme.TabBackground = theme.Background
	}
	if !seen["tabtext"] {
		theme.TabText = theme.Muted
	}
	if !seen["tabactivebackground"] {
		theme.TabActiveBackground = theme.Selection
	}
	if !seen["tabactivetext"] {
		theme.TabActiveText = theme.Accent
	}
	if !seen["import"] {
		theme.Import = theme.String
	}
	if !seen["declaration"] {
		theme.Declaration = theme.Keyword
	}
	if !seen["function"] {
		theme.Function = theme.Type
	}
	if !seen["delimiter"] {
		theme.Delimiter = theme.Foreground
	}
	if !seen["plain"] {
		theme.Plain = theme.Foreground
	}
	if !seen["delimiter2"] {
		theme.Delimiter2 = theme.Delimiter
	}
	if !seen["delimiter3"] {
		theme.Delimiter3 = theme.Delimiter
	}
	if !seen["constant"] {
		theme.Constant = theme.Number
	}
	borders, err := resolveBorderCharacters(document.Borders)
	if err != nil {
		return Theme{}, err
	}
	theme.Borders = borders
	return theme, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("theme file contains more than one JSON value")
		}
		return err
	}
	return nil
}

func resolveBorderCharacters(document borderDocument) (BorderCharacters, error) {
	characters := BorderCharacters{}
	statusSeparator := document.StatusSeparator
	if statusSeparator == "" {
		statusSeparator = ""
	}
	values := []struct {
		name   string
		value  string
		target *rune
	}{
		{name: "status_separator", value: statusSeparator, target: &characters.StatusSeparator},
		{name: "separator.horizontal", value: document.Separator.Horizontal, target: &characters.Separator.Horizontal},
		{name: "separator.vertical", value: document.Separator.Vertical, target: &characters.Separator.Vertical},
		{name: "corners.top_left", value: document.Corners.TopLeft, target: &characters.Corners.TopLeft},
		{name: "corners.top_right", value: document.Corners.TopRight, target: &characters.Corners.TopRight},
		{name: "corners.bottom_left", value: document.Corners.BottomLeft, target: &characters.Corners.BottomLeft},
		{name: "corners.bottom_right", value: document.Corners.BottomRight, target: &characters.Corners.BottomRight},
		{name: "lines.horizontal", value: document.Lines.Horizontal, target: &characters.Lines.Horizontal},
		{name: "lines.vertical", value: document.Lines.Vertical, target: &characters.Lines.Vertical},
	}
	for _, current := range values {
		if utf8.RuneCountInString(current.value) != 1 {
			return BorderCharacters{}, fmt.Errorf("border character %s must be one character", current.name)
		}
		*current.target, _ = utf8.DecodeRuneInString(current.value)
	}
	return characters, nil
}

func (t Theme) syntaxStyle(kind syntax.Kind) terminal.Style {
	foreground := t.Plain
	switch kind {
	case syntax.Comment:
		foreground = t.Comment
	case syntax.Keyword:
		foreground = t.Keyword
	case syntax.String:
		foreground = t.String
	case syntax.Number:
		foreground = t.Number
	case syntax.Type:
		foreground = t.Type
	case syntax.Import:
		foreground = t.Import
	case syntax.Declaration:
		foreground = t.Declaration
	case syntax.Function:
		foreground = t.Function
	case syntax.Delimiter:
		foreground = t.Delimiter
	case syntax.Delimiter2:
		foreground = t.Delimiter2
	case syntax.Delimiter3:
		foreground = t.Delimiter3
	case syntax.Constant:
		foreground = t.Constant
	case syntax.Added:
		foreground = t.String
	case syntax.Removed:
		foreground = t.Error
	}
	return terminal.Style{Foreground: foreground, Background: t.Background}
}

func (t *Theme) setColor(name, value string) error {
	parsed, err := terminal.ParseColor(value)
	if err != nil {
		return fmt.Errorf("theme color %s: %w", name, err)
	}
	switch normalizeColorName(name) {
	case "background":
		t.Background = parsed
	case "foreground":
		t.Foreground = parsed
	case "muted":
		t.Muted = parsed
	case "selection":
		t.Selection = parsed
	case "selectiontext":
		t.SelectionText = parsed
	case "tabbackground":
		t.TabBackground = parsed
	case "tabtext":
		t.TabText = parsed
	case "tabactivebackground":
		t.TabActiveBackground = parsed
	case "tabactivetext":
		t.TabActiveText = parsed
	case "cursor":
		t.Cursor = parsed
	case "status":
		t.Status = parsed
	case "statustext":
		t.StatusText = parsed
	case "accent":
		t.Accent = parsed
	case "success":
		t.Success = parsed
	case "warning":
		t.Warning = parsed
	case "danger":
		t.Danger = parsed
	case "error":
		t.Error = parsed
	case "diagnostic":
		t.Diagnostic = parsed
	case "panel":
		t.Panel = parsed
	case "panelborder":
		t.PanelBorder = parsed
	case "diagnosticbackground":
		t.DiagnosticBackground = parsed
	case "comment":
		t.Comment = parsed
	case "plain":
		t.Plain = parsed
	case "keyword":
		t.Keyword = parsed
	case "string":
		t.String = parsed
	case "number":
		t.Number = parsed
	case "type":
		t.Type = parsed
	case "import":
		t.Import = parsed
	case "declaration":
		t.Declaration = parsed
	case "function":
		t.Function = parsed
	case "delimiter":
		t.Delimiter = parsed
	case "delimiter2":
		t.Delimiter2 = parsed
	case "delimiter3":
		t.Delimiter3 = parsed
	case "constant":
		t.Constant = parsed
	default:
		return fmt.Errorf("unknown theme color %q", name)
	}
	return nil
}

func normalizeColorName(name string) string {
	return strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(name))
}
