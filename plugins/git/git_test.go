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
}

func (h *testHost) Root() string {
	return h.root
}

func (h *testHost) CurrentPath() string {
	return h.currentPath
}

func (h *testHost) RegisterCommand(name string, command plugin.Command) error {
	h.commands[name] = command
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
