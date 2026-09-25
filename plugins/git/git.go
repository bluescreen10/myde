// Package git provides Git commands for myde.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bluescreen10/myde/plugin"
)

const (
	unstagedKind = "unstaged"
	stagedKind   = "staged"
)

type gitPlugin struct {
	host plugin.Host
}

type fileState struct {
	path     string
	status   string
	staged   bool
	unstaged bool
}

// New returns the Git plugin.
func New() plugin.Plugin {
	return &gitPlugin{}
}

func (g *gitPlugin) Name() string {
	return "git"
}

func (g *gitPlugin) Load(host plugin.Host) error {
	g.host = host
	commands := []struct {
		name    string
		handler plugin.Command
	}{
		{name: "git.diff", handler: g.diffCommand},
		{name: "git.commit", handler: g.commitCommand},
		{name: "git.stage", handler: g.stageCommand},
	}
	for _, command := range commands {
		if err := host.RegisterCommand(command.name, command.handler); err != nil {
			return err
		}
	}
	return nil
}

func (g *gitPlugin) stageCommand(arguments string) error {
	return g.showStagePanel(strings.TrimSpace(arguments))
}

func (g *gitPlugin) showStagePanel(selected string) error {
	states, err := g.status()
	if err != nil {
		return err
	}
	unstaged := make([]plugin.SidebarItem, 0)
	staged := make([]plugin.SidebarItem, 0)
	for _, state := range states {
		if state.unstaged {
			unstaged = append(unstaged, plugin.SidebarItem{
				Label: state.path, Detail: state.status, Value: state.path, Kind: unstagedKind,
			})
		}
		if state.staged {
			staged = append(staged, plugin.SidebarItem{
				Label: state.path, Detail: state.status, Value: state.path, Kind: stagedKind,
			})
		}
	}
	g.host.OpenSidebar(plugin.Sidebar{
		Title: "Git Changes",
		Sections: []plugin.SidebarSection{
			{Title: "Unstaged", Items: unstaged},
			{Title: "Staged", Items: staged},
		},
		SelectedValue: selected,
		Help: []plugin.KeyHelp{
			{Key: "+", Label: "stage"},
			{Key: "-", Label: "unstage"},
			{Key: "Enter", Label: "diff"},
		},
		OnAction: g.handleStageAction,
	})
	return nil
}

func (g *gitPlugin) handleStageAction(action plugin.Action, item plugin.SidebarItem) error {
	switch action {
	case plugin.Add:
		if item.Kind != unstagedKind {
			return nil
		}
		if _, err := g.run("add", "--", item.Value); err != nil {
			return err
		}
		if err := g.showStagePanel(item.Value); err != nil {
			return err
		}
		g.host.SetMessage("staged " + item.Value)
	case plugin.Remove:
		if item.Kind != stagedKind {
			return nil
		}
		if _, err := g.run("reset", "-q", "--", item.Value); err != nil {
			return err
		}
		if err := g.showStagePanel(item.Value); err != nil {
			return err
		}
		g.host.SetMessage("unstaged " + item.Value)
	case plugin.Activate:
		return g.openDiff(item.Kind == stagedKind, item.Value, item.Detail == "??")
	}
	return nil
}

func (g *gitPlugin) diffCommand(arguments string) error {
	arguments = strings.TrimSpace(arguments)
	staged := false
	if arguments == "staged" {
		staged = true
		arguments = ""
	} else if strings.HasPrefix(arguments, "staged ") {
		staged = true
		arguments = strings.TrimSpace(strings.TrimPrefix(arguments, "staged "))
	}
	path := arguments
	if path == "" {
		path = g.host.CurrentPath()
	}
	return g.openDiff(staged, path, false)
}

func (g *gitPlugin) openDiff(staged bool, path string, untracked bool) error {
	arguments := []string{"diff"}
	if staged {
		arguments = append(arguments, "--staged")
	}
	if path != "" {
		arguments = append(arguments, "--", path)
	}
	var output []byte
	var err error
	if untracked {
		output, err = g.runDiff("diff", "--no-index", "--", os.DevNull, path)
	} else {
		output, err = g.run(arguments...)
	}
	if err != nil {
		return err
	}
	if len(output) == 0 {
		output = []byte("(no changes)\n")
	}
	label := "workspace"
	if path != "" {
		label = path
		if relative, relativeErr := filepath.Rel(g.host.Root(), path); relativeErr == nil {
			label = relative
		}
	}
	kind := "unstaged"
	if staged {
		kind = "staged"
	}
	name := fmt.Sprintf("git %s · %s.diff", kind, filepath.ToSlash(label))
	g.host.OpenReadOnlyBuffer(name, output)
	return nil
}

func (g *gitPlugin) commitCommand(arguments string) error {
	message := strings.TrimSpace(arguments)
	if message == "" {
		g.host.Prompt("Git Commit Message", g.commit)
		return nil
	}
	return g.commit(message)
}

func (g *gitPlugin) commit(message string) error {
	output, err := g.run("commit", "-m", message)
	if err != nil {
		return err
	}
	g.host.CloseSidebar()
	text := strings.TrimSpace(string(output))
	if text == "" {
		text = "commit created"
	}
	g.host.SetMessage(firstLine(text))
	return nil
}

func (g *gitPlugin) status() ([]fileState, error) {
	output, err := g.run("status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	states := make([]fileState, 0)
	fields := bytes.Split(output, []byte{0})
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		if len(field) < 4 {
			continue
		}
		indexStatus := field[0]
		worktreeStatus := field[1]
		path := string(field[3:])
		states = append(states, fileState{
			path:     path,
			status:   string(field[:2]),
			staged:   indexStatus != ' ' && indexStatus != '?',
			unstaged: worktreeStatus != ' ' || indexStatus == '?',
		})
		if indexStatus == 'R' || indexStatus == 'C' || worktreeStatus == 'R' || worktreeStatus == 'C' {
			index++
		}
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].path < states[j].path
	})
	return states, nil
}

func (g *gitPlugin) run(arguments ...string) ([]byte, error) {
	command := exec.Command("git", arguments...)
	command.Dir = g.host.Root()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", arguments[0], message)
	}
	return stdout.Bytes(), nil
}

func (g *gitPlugin) runDiff(arguments ...string) ([]byte, error) {
	command := exec.Command("git", arguments...)
	command.Dir = g.host.Root()
	output, err := command.CombinedOutput()
	var exitError *exec.ExitError
	if err != nil && (!errors.As(err, &exitError) || exitError.ExitCode() != 1) {
		return nil, fmt.Errorf("git %s: %s", arguments[0], strings.TrimSpace(string(output)))
	}
	return output, nil
}

func firstLine(value string) string {
	line, _, _ := strings.Cut(value, "\n")
	return line
}
