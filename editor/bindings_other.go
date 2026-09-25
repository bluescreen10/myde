//go:build !darwin

package editor

func platformBindings() map[string]string {
	return map[string]string{
		"ctrl-f":       "search.buffer",
		"ctrl-s":       "file.save",
		"ctrl-v":       "edit.paste",
		"ctrl-x":       "edit.cut",
		"ctrl-y":       "edit.redo",
		"ctrl-z":       "edit.undo",
	}
}
