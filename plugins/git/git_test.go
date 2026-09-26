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
	view        plugin.View
	viewHandle  *testViewHandle
	newViews    int
}

type testViewHandle struct {
	host      *testHost
	alive     bool
	showCount int
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

func (h *testHost) NewView(view plugin.View) plugin.ViewHandle {
	h.view = view
	h.newViews++
	h.viewHandle = &testViewHandle{host: h, alive: true}
	return h.viewHandle
}

func (h *testViewHandle) Show() bool {
	if !h.alive {
		return false
	}
	h.showCount++
	return true
}

func (h *testViewHandle) Update(view plugin.View) bool {
	if !h.alive {
		return false
	}
	h.host.view = view
	return true
}

func (h *testViewHandle) Destroy() {
	h.alive = false
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
	changes := changesList(t, host.view)
	unstaged := listItem(t, changes, "Unstaged", "main.go")
	if host.view.Layout.Direction != plugin.LayoutHorizontal || len(host.view.Layout.Panes) != 2 {
		t.Fatalf("git stage layout = %+v", host.view.Layout)
	}
	if changes.OnSelect == nil {
		t.Fatal("git stage changes list has no preview callback")
	}
	preview, err := changes.OnSelect(unstaged)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Syntax != "diff" || !bytes.Contains(preview.Content, []byte("+func main() {}")) {
		t.Fatalf("unstaged preview = %+v", preview)
	}
	if err := changes.OnAction(plugin.Add, unstaged); err != nil {
		t.Fatal(err)
	}
	changes = changesList(t, host.view)
	staged := listItem(t, changes, "Staged", "main.go")
	if err := changes.OnAction(plugin.Activate, staged); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(host.bufferName, ".diff") || !bytes.Contains(host.buffer, []byte("+func main() {}")) {
		t.Fatalf("diff buffer = %q %q", host.bufferName, host.buffer)
	}

	if err := host.commands["git.stage"]("main.go"); err != nil {
		t.Fatal(err)
	}
	if host.newViews != 1 || host.viewHandle.showCount != 1 {
		t.Fatalf("second git.stage created %d views and showed existing %d times", host.newViews, host.viewHandle.showCount)
	}
	changes = changesList(t, host.view)
	staged = listItem(t, changes, "Staged", "main.go")
	if err := changes.OnAction(plugin.Remove, staged); err != nil {
		t.Fatal(err)
	}
	changes = changesList(t, host.view)
	listItem(t, changes, "Unstaged", "main.go")

	unstaged = listItem(t, changes, "Unstaged", "main.go")
	if err := changes.OnAction(plugin.Add, unstaged); err != nil {
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
	if host.viewHandle.alive {
		t.Fatal("committing did not destroy the Git Stage view")
	}
}

func TestStageViewSelectsNextFileAndCanRefresh(t *testing.T) {
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
	changes := changesList(t, host.view)
	first := listItem(t, changes, "Unstaged", "a.txt")
	if first.Detail != "M" || first.DetailTone != plugin.ToneWarning {
		t.Fatalf("modified status = %+v", first)
	}
	if host.view.OnRefresh == nil || host.view.RefreshInterval <= 0 {
		t.Fatal("stage view has no background refresh")
	}
	if err := changes.OnAction(plugin.Add, first); err != nil {
		t.Fatal(err)
	}
	changes = changesList(t, host.view)
	if changes.SelectedKind != "unstaged" || changes.SelectedValue != "b.txt" {
		t.Fatalf("selection after staging a.txt = %s/%s", changes.SelectedKind, changes.SelectedValue)
	}
	second := listItem(t, changes, "Unstaged", "b.txt")
	if err := changes.OnAction(plugin.Add, second); err != nil {
		t.Fatal(err)
	}
	changes = changesList(t, host.view)
	stagedA := listItem(t, changes, "Staged", "a.txt")
	if err := changes.OnAction(plugin.Remove, stagedA); err != nil {
		t.Fatal(err)
	}
	changes = changesList(t, host.view)
	if changes.SelectedKind != "staged" || changes.SelectedValue != "b.txt" {
		t.Fatalf("selection after unstaging a.txt = %s/%s", changes.SelectedKind, changes.SelectedValue)
	}

	if err := os.WriteFile(filepath.Join(root, "c.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refreshed, err := host.view.OnRefresh()
	if err != nil {
		t.Fatal(err)
	}
	added := listItem(t, changesList(t, refreshed), "Unstaged", "c.txt")
	if added.Detail != "A" || added.DetailTone != plugin.ToneSuccess {
		t.Fatalf("added status = %+v", added)
	}
}

func changesList(t *testing.T, view plugin.View) *plugin.List {
	t.Helper()
	for _, pane := range view.Layout.Panes {
		if pane.ID == "changes" && pane.List != nil {
			return pane.List
		}
	}
	t.Fatal("changes list not found")
	return nil
}

func listItem(t *testing.T, list *plugin.List, sectionTitle, path string) plugin.ListItem {
	t.Helper()
	for _, section := range list.Sections {
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
	return plugin.ListItem{}
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
