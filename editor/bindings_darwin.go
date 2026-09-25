//go:build darwin

package editor

func platformBindings() map[string]string {
	return map[string]string{
		"ctrl-c":     "edit.copy",
		"alt-d":      "cursor.add-next-match",
		"ctrl-up":    "cursor.file-start",
		"ctrl-down":  "cursor.file-end",
		"ctrl-left":  "cursor.line-start",
		"ctrl-right": "cursor.line-end",
		"ctrl-f":     "search.buffer",
		"ctrl-v":     "edit.paste",
		"ctrl-x":     "edit.cut",
		"ctrl-y":     "edit.redo",
		"ctrl-z":     "edit.undo",
		"ctrl-s":      "file.save",
	}
}
