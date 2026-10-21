package git_test

import (
	"github.com/bluescreen10/myde/plugin"
	gitplugin "github.com/bluescreen10/myde/plugins/git"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bluescreen10/myde/ui"
)

func TestDiffWidgetOwnsDiffStyling(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.invalid")
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("old value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "a.txt")
	runGit(t, root, "commit", "-qm", "initial")
	if err := os.WriteFile(path, []byte("new value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	host := &testHost{root: root, commands: make(map[string]plugin.Command)}
	if err := gitplugin.New().Load(host); err != nil {
		t.Fatal(err)
	}
	if err := host.commands["git.stage"](""); err != nil {
		t.Fatal(err)
	}
	list := changesList(t, host.view)
	widget, err := list.OnSelect(listItem(t, list, "Unstaged", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	content := widget.RenderWidget()
	if content.Title == "" || len(content.Lines) < 4 {
		t.Fatalf("content = %+v", content)
	}
	if content.Lines[0].Style.Foreground != ui.ToneMuted {
		t.Fatalf("metadata style = %+v", content.Lines[0].Style)
	}
	if content.Lines[len(content.Lines)-3].Style.Foreground != ui.ToneAccent ||
		content.Lines[len(content.Lines)-3].Style.Background != ui.ToneAccent {
		t.Fatalf("hunk style = %+v", content.Lines[len(content.Lines)-3].Style)
	}
	assertChangedDiffLine(t, content.Lines[len(content.Lines)-2], ui.ToneDanger)
	assertChangedDiffLine(t, content.Lines[len(content.Lines)-1], ui.ToneSuccess)
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
