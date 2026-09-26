package editor

import (
	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/syntax"
)

// editorBuffer owns the editor-specific state associated with one text buffer.
type editorBuffer struct {
	text        *buffer.Buffer
	highlighter *syntax.Highlighter
	diagnostics []diagnostic
	breakpoints map[int]bool
	mode        string
	lspOpened   bool
	terminal    *shellBuffer
	view        *viewPanel
}

func (a *App) newEditorBuffer(text *buffer.Buffer) *editorBuffer {
	mode := a.modeForPath(text.Path())
	return &editorBuffer{
		text:        text,
		highlighter: a.highlighterForMode(mode),
		breakpoints: make(map[int]bool),
		mode:        mode,
	}
}

func (a *App) currentEditorBuffer() *editorBuffer {
	return a.buffers[a.active]
}

func (a *App) editorBufferFor(text *buffer.Buffer) *editorBuffer {
	for _, current := range a.buffers {
		if current.text == text {
			return current
		}
	}
	return nil
}
