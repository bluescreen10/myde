//go:build darwin

package editor

func platformBindings() map[string]string {
	return map[string]string{
		"ctrl-s":        "search.buffer",
		"ctrl-y":        "edit.redo",
		"ctrl-z":        "edit.undo",
		"super-f":       "search.buffer",
		"super-s":       "file.save",
		"super-shift-z": "edit.redo",
		"super-v":       "edit.paste",
		"super-x":       "edit.cut",
		"super-y":       "edit.redo",
		"super-z":       "edit.undo",
	}
}
