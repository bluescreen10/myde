package git_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluescreen10/myde/plugin"
	gitplugin "github.com/bluescreen10/myde/plugins/git"
)

type testHost struct {
	root        string
	currentPath string
	commands    map[string]plugin.Command
	sidebar     plugin.Sidebar
	prompt      func(string) error
	bufferName  string
	buffer      []byte
	message     string
	modes       []plugin.Mode
}

func (h *testHost) Root() string {
	return h.root
}

func (h *testHost) CurrentPath() string {
	return h.currentPath
}

func (h *testHost) CurrentDocument() plugin.Document {
	return plugin.Document{Path: h.currentPath, Content: append([]byte(nil), h.buffer...)}
}

func (h *testHost) ReplaceCurrentDocument(content []byte) error {
	h.buffer = append([]byte(nil), content...)
	return nil
}

func (h *testHost) RegisterCommand(name string, command plugin.Command) error {
	h.commands[name] = command
	return nil
}

func (h *testHost) RegisterMode(mode plugin.Mode) error {
	h.modes = append(h.modes, mode)
	return nil
}

func (h *testHost) OpenSidebar(sidebar plugin.Sidebar) {
	h.sidebar = sidebar
}

func (h *testHost) CloseSidebar() {
	h.sidebar = plugin.Sidebar{}
}

func (h *testHost) OpenReadOnlyBuffer(name string, content []byte) {
	h.bufferName = name
	h.buffer = append([]byte(nil), content...)
}

func (h *testHost) Prompt(title string, submit func(string) error) {
	h.prompt = submit
}

func (h *testHost) SetMessage(message string) {
	h.message = message
}

func TestPluginStagesDiffsUnstagesAndCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.name", "Myde Test")
	runGit(t, root, "config", "user.email", "myde@example.invalid")
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "main.go")
	runGit(t, root, "commit", "-qm", "initial")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	host := &testHost{
		root: root, currentPath: path, commands: make(map[string]plugin.Command),
	}
	loaded := gitplugin.New()
	if err := loaded.Load(host); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git.diff", "git.commit", "git.stage"} {
		if host.commands[name] == nil {
			t.Fatalf("command %q was not registered", name)
		}
	}

	if err := host.commands["git.stage"](""); err != nil {
		t.Fatal(err)
	}
	unstaged := sidebarItem(t, host.sidebar, "Unstaged", "main.go")
	if !host.sidebar.FullScreen || host.sidebar.OnPreview == nil {
		t.Fatal("git stage did not open the full-screen review view")
	}
	preview, err := host.sidebar.OnPreview(unstaged)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Syntax != "diff" || !bytes.Contains(preview.Content, []byte("+func main() {}")) {
		t.Fatalf("unstaged preview = %+v", preview)
	}
	if err := host.sidebar.OnAction(plugin.Add, unstaged); err != nil {
		t.Fatal(err)
	}
	staged := sidebarItem(t, host.sidebar, "Staged", "main.go")
	if err := host.sidebar.OnAction(plugin.Activate, staged); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(host.bufferName, ".diff") || !bytes.Contains(host.buffer, []byte("+func main() {}")) {
		t.Fatalf("diff buffer = %q %q", host.bufferName, host.buffer)
	}

	if err := host.commands["git.stage"]("main.go"); err != nil {
		t.Fatal(err)
	}
	staged = sidebarItem(t, host.sidebar, "Staged", "main.go")
	if err := host.sidebar.OnAction(plugin.Remove, staged); err != nil {
		t.Fatal(err)
	}
	sidebarItem(t, host.sidebar, "Unstaged", "main.go")

	unstaged = sidebarItem(t, host.sidebar, "Unstaged", "main.go")
	if err := host.sidebar.OnAction(plugin.Add, unstaged); err != nil {
		t.Fatal(err)
	}
	if err := host.commands["git.commit"](""); err != nil {
		t.Fatal(err)
	}
	if host.prompt == nil {
		t.Fatal("git.commit did not open a prompt")
	}
	if err := host.prompt("add main"); err != nil {
		t.Fatal(err)
	}
	if got := runGit(t, root, "log", "-1", "--pretty=%s"); strings.TrimSpace(got) != "add main" {
		t.Fatalf("commit subject = %q, want add main", got)
	}
}

func TestStagePanelSelectsNextFileAndCanRefresh(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.name", "Myde Test")
	runGit(t, root, "config", "user.email", "myde@example.invalid")
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, root, "add", "a.txt", "b.txt")
	runGit(t, root, "commit", "-qm", "initial")
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("after\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	host := &testHost{root: root, commands: make(map[string]plugin.Command)}
	if err := gitplugin.New().Load(host); err != nil {
		t.Fatal(err)
	}
	if err := host.commands["git.stage"](""); err != nil {
		t.Fatal(err)
	}
	first := sidebarItem(t, host.sidebar, "Unstaged", "a.txt")
	if first.Detail != "M" || first.DetailTone != plugin.ToneWarning {
		t.Fatalf("modified status = %+v", first)
	}
	if host.sidebar.OnRefresh == nil || host.sidebar.RefreshInterval <= 0 {
		t.Fatal("stage panel has no background refresh")
	}
	if err := host.sidebar.OnAction(plugin.Add, first); err != nil {
		t.Fatal(err)
	}
	if host.sidebar.SelectedKind != "unstaged" || host.sidebar.SelectedValue != "b.txt" {
		t.Fatalf("selection after staging a.txt = %s/%s", host.sidebar.SelectedKind, host.sidebar.SelectedValue)
	}
	second := sidebarItem(t, host.sidebar, "Unstaged", "b.txt")
	if err := host.sidebar.OnAction(plugin.Add, second); err != nil {
		t.Fatal(err)
	}
	stagedA := sidebarItem(t, host.sidebar, "Staged", "a.txt")
	if err := host.sidebar.OnAction(plugin.Remove, stagedA); err != nil {
		t.Fatal(err)
	}
	if host.sidebar.SelectedKind != "staged" || host.sidebar.SelectedValue != "b.txt" {
		t.Fatalf("selection after unstaging a.txt = %s/%s", host.sidebar.SelectedKind, host.sidebar.SelectedValue)
	}

	if err := os.WriteFile(filepath.Join(root, "c.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refreshed, err := host.sidebar.OnRefresh()
	if err != nil {
		t.Fatal(err)
	}
	added := sidebarItem(t, refreshed, "Unstaged", "c.txt")
	if added.Detail != "A" || added.DetailTone != plugin.ToneSuccess {
		t.Fatalf("added status = %+v", added)
	}
}

func sidebarItem(t *testing.T, sidebar plugin.Sidebar, sectionTitle, path string) plugin.SidebarItem {
	t.Helper()
	for _, section := range sidebar.Sections {
		if section.Title != sectionTitle {
			continue
		}
		for _, item := range section.Items {
			if item.Value == path {
				return item
			}
		}
	}
	t.Fatalf("%s item %q not found", sectionTitle, path)
	return plugin.SidebarItem{}
}

func runGit(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}
