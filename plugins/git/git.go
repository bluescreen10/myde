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
	"time"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/ui"
)

const (
	unstagedKind = "unstaged"
	stagedKind   = "staged"
)

type gitPlugin struct {
	host        plugin.Host
	stageHandle ui.ViewHandle
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
	return g.showStageView("", strings.TrimSpace(arguments))
}

func (g *gitPlugin) showStageView(selectedKind, selectedValue string) error {
	states, err := g.status()
	if err != nil {
		return err
	}
	view := g.stageView(states, selectedKind, selectedValue)
	if g.stageHandle != nil && g.stageHandle.Update(view) {
		g.stageHandle.Show()
		return nil
	}
	g.stageHandle = g.host.NewView(view)
	return nil
}

func (g *gitPlugin) stageView(states []fileState, selectedKind, selectedValue string) ui.View {
	unstaged := make([]ui.ListItem, 0)
	staged := make([]ui.ListItem, 0)
	for _, state := range states {
		if state.unstaged {
			status, tone := displayStatus(state, false)
			unstaged = append(unstaged, ui.ListItem{
				Label: state.path, Detail: status, DetailTone: tone, Value: state.path, Kind: unstagedKind,
				Data: state.status,
			})
		}
		if state.staged {
			status, tone := displayStatus(state, true)
			staged = append(staged, ui.ListItem{
				Label: state.path, Detail: status, DetailTone: tone, Value: state.path, Kind: stagedKind,
				Data: state.status,
			})
		}
	}
	return ui.View{
		Title: "Git Stage",
		Layout: ui.Layout{
			Direction: ui.LayoutHorizontal,
			Panes: []ui.Pane{
				{
					ID: "changes", Title: "Git Changes", Weight: 1,
					List: &ui.List{
						Sections: []ui.ListSection{
							{Title: "Unstaged", Items: unstaged},
							{Title: "Staged", Items: staged},
						},
						SelectedValue: selectedValue,
						SelectedKind:  selectedKind,
						PreviewPane:   "diff",
						Help: []ui.KeyHelp{
							{Key: "+", Label: "stage"},
							{Key: "-", Label: "unstage"},
						},
						OnAction: g.handleStageAction,
						OnSelect: g.previewDiff,
					},
				},
				{ID: "diff", Title: "Diff", Weight: 2},
			},
		},
		RefreshInterval: time.Second,
		OnRefresh: func() (ui.View, error) {
			updated, err := g.status()
			if err != nil {
				return ui.View{}, err
			}
			return g.stageView(updated, "", ""), nil
		},
	}
}

func (g *gitPlugin) handleStageAction(action ui.Action, item ui.ListItem) error {
	switch action {
	case ui.Add:
		if item.Kind != unstagedKind {
			return nil
		}
		if _, err := g.run("add", "--", item.Value); err != nil {
			return err
		}
		states, err := g.status()
		if err != nil {
			return err
		}
		kind, selected := nextSelection(states, unstagedKind, item.Value)
		if err := g.updateStageView(states, kind, selected); err != nil {
			return err
		}
		g.host.SetMessage("staged " + item.Value)
	case ui.Remove:
		if item.Kind != stagedKind {
			return nil
		}
		if _, err := g.run("reset", "-q", "--", item.Value); err != nil {
			return err
		}
		states, err := g.status()
		if err != nil {
			return err
		}
		kind, selected := nextSelection(states, stagedKind, item.Value)
		if err := g.updateStageView(states, kind, selected); err != nil {
			return err
		}
		g.host.SetMessage("unstaged " + item.Value)
	case ui.Activate:
		return g.openDiff(item.Kind == stagedKind, item.Value, item.Data == "??")
	}
	return nil
}

func (g *gitPlugin) updateStageView(states []fileState, selectedKind, selectedValue string) error {
	view := g.stageView(states, selectedKind, selectedValue)
	if g.stageHandle != nil && g.stageHandle.Update(view) {
		return nil
	}
	g.stageHandle = g.host.NewView(view)
	return nil
}

func displayStatus(state fileState, staged bool) (string, ui.Tone) {
	status := byte(' ')
	if staged {
		status = state.status[0]
	} else {
		status = state.status[1]
		if state.status == "??" {
			status = 'A'
		}
	}
	switch status {
	case 'A', '?':
		return "A", ui.ToneSuccess
	case 'D':
		return "D", ui.ToneDanger
	case 'R':
		return "R", ui.ToneWarning
	default:
		return "M", ui.ToneWarning
	}
}

func nextSelection(states []fileState, kind, after string) (string, string) {
	candidates := make([]string, 0, len(states))
	for _, state := range states {
		if kind == unstagedKind && state.unstaged || kind == stagedKind && state.staged {
			candidates = append(candidates, state.path)
		}
	}
	for _, path := range candidates {
		if path > after {
			return kind, path
		}
	}
	if len(candidates) > 0 {
		return kind, candidates[len(candidates)-1]
	}
	other := unstagedKind
	if kind == unstagedKind {
		other = stagedKind
	}
	for _, state := range states {
		if other == unstagedKind && state.unstaged || other == stagedKind && state.staged {
			return other, state.path
		}
	}
	return "", ""
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
	output, err := g.diff(staged, path, untracked)
	if err != nil {
		return err
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

func (g *gitPlugin) previewDiff(item ui.ListItem) (ui.Widget, error) {
	staged := item.Kind == stagedKind
	output, err := g.diff(staged, item.Value, item.Data == "??")
	if err != nil {
		return nil, err
	}
	kind := "Unstaged"
	if staged {
		kind = "Staged"
	}
	return newDiffWidget(kind+" · "+filepath.ToSlash(item.Value), output), nil
}

func (g *gitPlugin) diff(staged bool, path string, untracked bool) ([]byte, error) {
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
		return nil, err
	}
	if len(output) == 0 {
		output = []byte("(no changes)\n")
	}
	return output, nil
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
	if g.stageHandle != nil {
		g.stageHandle.Destroy()
		g.stageHandle = nil
	}
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
