//go:build darwin

package editor

func platformBindings() map[string]string {
	return map[string]string{
		"ctrl-a":        "cursor.line-start",
		"ctrl-c":        "edit.copy",
		"ctrl-d":        "cursor.add-next-match",
		"ctrl-down":     "cursor.file-end",
		"ctrl-e":        "cursor.line-end",
		"ctrl-left":     "cursor.line-start",
		"ctrl-right":    "cursor.line-end",
		"ctrl-s":        "search.buffer",
		"ctrl-up":       "cursor.file-start",
		"ctrl-v":        "edit.paste",
		"ctrl-x":        "edit.cut",
		"ctrl-y":        "edit.redo",
		"ctrl-z":        "edit.undo",
		"super-f":       "search.buffer",
		"super-s":       "file.save",
		"super-shift-z": "edit.redo",
		"super-y":       "edit.redo",
		"super-z":       "edit.undo",
	}
}
