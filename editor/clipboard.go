package editor

import (
	"fmt"
	"strings"

	"github.com/bluescreen10/myde/buffer"
)

func (a *App) copy(arguments string) error {
	selections := selectedText(a.current())
	if len(selections) == 0 {
		a.message = "nothing selected"
		return nil
	}
	if err := writeSystemClipboard(strings.Join(selections, "\n")); err != nil {
		return err
	}
	a.message = "copied selection"
	return nil
}

func (a *App) cut(arguments string) error {
	current := a.current()
	if current.IsReadOnly() {
		return fmt.Errorf("%s is read-only", current.Name())
	}
	selections := selectedText(current)
	if len(selections) == 0 {
		a.message = "nothing selected"
		return nil
	}
	if a.currentEditorBuffer().terminal != nil {
		return fmt.Errorf("terminal output cannot be cut")
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
	if shell := a.currentEditorBuffer().terminal; shell != nil {
		if !shell.running || shell.pty == nil {
			return fmt.Errorf("terminal shell is not running")
		}
		if _, err := shell.pty.Write([]byte(text)); err != nil {
			return fmt.Errorf("terminal input: %w", err)
		}
		a.message = "pasted"
		return nil
	}
	if a.current().IsReadOnly() {
		return fmt.Errorf("%s is read-only", a.current().Name())
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
