package editor

import "github.com/bluescreen10/myde/terminal"

// editTextInput is shared by prompts, palettes, and sidebar query fields.
func (a *App) editTextInput(query *[]rune, cursor *int, event terminal.Event) (handled, changed bool) {
	*cursor = min(max(0, *cursor), len(*query))
	command := a.bindings[keyName(event)]
	switch command {
	case "cursor.line-start", "cursor.file-start":
		*cursor = 0
		return true, false
	case "cursor.line-end", "cursor.file-end":
		*cursor = len(*query)
		return true, false
	}
	switch event.Key {
	case terminal.KeyHome:
		*cursor = 0
	case terminal.KeyEnd:
		*cursor = len(*query)
	case terminal.KeyLeft:
		if event.Control || event.Super {
			*cursor = 0
		} else {
			*cursor = max(0, *cursor-1)
		}
	case terminal.KeyRight:
		if event.Control || event.Super {
			*cursor = len(*query)
		} else {
			*cursor = min(len(*query), *cursor+1)
		}
	case terminal.KeyBackspace:
		if *cursor > 0 {
			*query = append((*query)[:*cursor-1], (*query)[*cursor:]...)
			*cursor -= 1
			changed = true
		}
	case terminal.KeyDelete:
		if *cursor < len(*query) {
			*query = append((*query)[:*cursor], (*query)[*cursor+1:]...)
			changed = true
		}
	case terminal.KeyRune:
		if event.Control || event.Alt || event.Super || event.Rune == 0 {
			return false, false
		}
		*query = append(*query, 0)
		copy((*query)[*cursor+1:], (*query)[*cursor:])
		(*query)[*cursor] = event.Rune
		*cursor += 1
		changed = true
	default:
		return false, false
	}
	return true, changed
}

// inputViewport keeps the insertion point visible, including for wide runes.
func inputViewport(query []rune, cursor, width int) (string, int) {
	cursor = min(max(0, cursor), len(query))
	width = max(1, width)
	start := cursor
	used := 0
	for start > 0 {
		next := terminal.RuneWidth(query[start-1])
		if used+next >= width {
			break
		}
		used += next
		start--
	}
	return truncate(string(query[start:]), width), used
}
