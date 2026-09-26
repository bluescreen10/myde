package git

import (
	"testing"

	"github.com/bluescreen10/myde/ui"
)

func TestDiffWidgetOwnsDiffStyling(t *testing.T) {
	widget := newDiffWidget("example.diff", []byte(
		"diff --git a/a.txt b/a.txt\n@@ -1 +1 @@\n-old value\n+new value\n",
	))
	content := widget.RenderWidget()
	if content.Title != "example.diff" || len(content.Lines) != 4 {
		t.Fatalf("content = %+v", content)
	}
	if content.Lines[0].Style.Foreground != ui.ToneMuted {
		t.Fatalf("metadata style = %+v", content.Lines[0].Style)
	}
	if content.Lines[1].Style.Foreground != ui.ToneAccent ||
		content.Lines[1].Style.Background != ui.ToneAccent {
		t.Fatalf("hunk style = %+v", content.Lines[1].Style)
	}
	assertChangedDiffLine(t, content.Lines[2], ui.ToneDanger)
	assertChangedDiffLine(t, content.Lines[3], ui.ToneSuccess)
}

func assertChangedDiffLine(t *testing.T, line ui.RichTextLine, tone ui.Tone) {
	t.Helper()
	if line.Style.Background != tone || line.Style.BackgroundIntensity != 22 {
		t.Fatalf("line style = %+v, want %s background", line.Style, tone)
	}
	if len(line.Spans) < 2 {
		t.Fatalf("line has no changed-text span: %+v", line)
	}
	found := false
	for _, span := range line.Spans {
		if span.Style.Background == tone && span.Style.BackgroundIntensity == 44 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("line has no strong %s span: %+v", tone, line)
	}
}
