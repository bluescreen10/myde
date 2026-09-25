//go:build darwin

package editor

func platformBindings() map[string]string {
	return map[string]string{
		"ctrl-a":            "cursor.line-start",
		"ctrl-e":            "cursor.line-end",
		"ctrl-s":            "search.buffer",
		"ctrl-y":            "edit.redo",
		"ctrl-z":            "edit.undo",
		"super-f":           "search.buffer",
		"super-left":        "cursor.line-start",
		"super-right":       "cursor.line-end",
		"super-s":           "file.save",
		"super-shift-down":  "cursor.file-end select",
		"super-shift-left":  "cursor.line-start select",
		"super-shift-right": "cursor.line-end select",
		"super-shift-up":    "cursor.file-start select",
		"super-shift-z":     "edit.redo",
		"super-up":          "cursor.file-start",
		"super-down":        "cursor.file-end",
		"super-v":           "edit.paste",
		"super-x":           "edit.cut",
		"super-y":           "edit.redo",
		"super-z":           "edit.undo",
	}
}
