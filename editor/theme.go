// Package editor implements the interactive terminal editor.
package editor

import (
	"fmt"
	"strings"

	"github.com/bluescreen10/myde/syntax"
	"github.com/bluescreen10/myde/terminal"
)

// Theme defines editor colors.
type Theme struct {
	Name string

	Background           terminal.Color
	Foreground           terminal.Color
	Muted                terminal.Color
	Selection            terminal.Color
	Cursor               terminal.Color
	Status               terminal.Color
	StatusText           terminal.Color
	Accent               terminal.Color
	Error                terminal.Color
	Diagnostic           terminal.Color
	Panel                terminal.Color
	PanelBorder          terminal.Color
	DiagnosticBackground terminal.Color

	Comment terminal.Color
	Keyword terminal.Color
	String  terminal.Color
	Number  terminal.Color
	Type    terminal.Color
}

// VSDark2026 is the default theme, based on VS Code's modern dark palette.
func VSDark2026() Theme {
	return Theme{
		Name:                 "vs-dark-2026",
		Background:           color("#1F1F1F"),
		Foreground:           color("#D4D4D4"),
		Muted:                color("#858585"),
		Selection:            color("#264F78"),
		Cursor:               color("#AEAFAD"),
		Status:               color("#007ACC"),
		StatusText:           color("#FFFFFF"),
		Accent:               color("#4FA8FF"),
		Error:                color("#F14C4C"),
		Diagnostic:           color("#F14C4C"),
		Panel:                color("#252526"),
		PanelBorder:          color("#5A5A5A"),
		DiagnosticBackground: color("#3A2427"),
		Comment:              color("#6A9955"),
		Keyword:              color("#C586C0"),
		String:               color("#CE9178"),
		Number:               color("#B5CEA8"),
		Type:                 color("#4EC9B0"),
	}
}

func (t Theme) syntaxStyle(kind syntax.Kind) terminal.Style {
	foreground := t.Foreground
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
		return err
	}
	switch strings.ToLower(name) {
	case "background":
		t.Background = parsed
	case "foreground":
		t.Foreground = parsed
	case "muted":
		t.Muted = parsed
	case "selection":
		t.Selection = parsed
	case "cursor":
		t.Cursor = parsed
	case "status":
		t.Status = parsed
	case "statustext":
		t.StatusText = parsed
	case "accent":
		t.Accent = parsed
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
	case "keyword":
		t.Keyword = parsed
	case "string":
		t.String = parsed
	case "number":
		t.Number = parsed
	case "type":
		t.Type = parsed
	default:
		return fmt.Errorf("unknown theme color %q", name)
	}
	return nil
}

func color(value string) terminal.Color {
	parsed, err := terminal.ParseColor(value)
	if err != nil {
		panic(err)
	}
	return parsed
}
