package editor

import (
	"fmt"
	"strings"

	"github.com/bluescreen10/myde/buffer"
)

func (a *App) cut(arguments string) error {
	current := a.current()
	selections := selectedText(current)
	if len(selections) == 0 {
		a.message = "nothing selected"
		return nil
	}
	if shell := a.terminals[current]; shell != nil {
		for _, cursor := range current.Cursors() {
			start := min(current.Offset(cursor.Anchor), current.Offset(cursor.Point))
			end := max(current.Offset(cursor.Anchor), current.Offset(cursor.Point))
			if start != end && start < shell.promptStart {
				return fmt.Errorf("terminal output cannot be cut")
			}
		}
	}
	if err := writeSystemClipboard(strings.Join(selections, "\n")); err != nil {
		return err
	}
	a.applyEdits(cursorEdits(current, nil, false, false))
	a.message = "cut selection"
	return nil
}

func (a *App) paste(arguments string) error {
	text, err := readSystemClipboard()
	if err != nil {
		return err
	}
	if text == "" {
		a.message = "clipboard is empty"
		return nil
	}
	if shell := a.terminals[a.current()]; shell != nil {
		if shell.running {
			return fmt.Errorf("terminal command is still running")
		}
		a.moveTerminalCursorToEnd(shell)
	}
	a.insert([]byte(text))
	a.message = "pasted"
	return nil
}

func selectedText(current *buffer.Buffer) []string {
	selections := make([]string, 0, len(current.Cursors()))
	for _, cursor := range current.Cursors() {
		start := current.Offset(cursor.Anchor)
		end := current.Offset(cursor.Point)
		if start > end {
			start, end = end, start
		}
		if start == end {
			continue
		}
		selections = append(selections, string(current.Slice(start, end)))
	}
	return selections
}
