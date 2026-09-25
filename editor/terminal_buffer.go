package editor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/terminal"
)

type shellBuffer struct {
	buffer      *buffer.Buffer
	workingPath string
	shellPath   string
	promptStart int
	running     bool
	cancel      context.CancelFunc
}

func (a *App) openTerminal(arguments string) error {
	for index, current := range a.buffers {
		if a.terminals[current] != nil {
			a.active = index
			a.ensureCursorVisible()
			return nil
		}
	}

	current := buffer.NewNamed("*terminal*")
	a.addBuffer(current)
	current.SetHistoryLimit(0)
	shell := &shellBuffer{buffer: current, workingPath: a.root, shellPath: userShell()}
	a.terminals[current] = shell
	a.appendTerminalPrompt(shell)
	a.topLine = 0
	return nil
}

func (a *App) handleTerminalEvent(event terminal.Event, shell *shellBuffer) error {
	if shell.running {
		if event.Key == terminal.KeyRune && event.Control && event.Rune == 'c' && shell.cancel != nil {
			shell.cancel()
			a.message = "terminal command interrupted"
			return nil
		}
		a.message = "terminal command is still running"
		return nil
	}
	switch event.Key {
	case terminal.KeyRune:
		if event.Control || event.Alt || event.Super || event.Rune == 0 {
			return nil
		}
		a.moveTerminalCursorToEnd(shell)
		a.insert([]byte(string(event.Rune)))
	case terminal.KeyEnter:
		return a.runTerminalCommand(shell)
	case terminal.KeyBackspace:
		a.moveTerminalCursorToEnd(shell)
		if shell.buffer.Len() > shell.promptStart {
			a.backspace()
		}
	case terminal.KeyUp:
		a.moveCursors(0, -1, event.Shift)
	case terminal.KeyDown:
		a.moveCursors(0, 1, event.Shift)
	case terminal.KeyLeft:
		a.moveCursors(-1, 0, event.Shift)
	case terminal.KeyRight:
		a.moveCursors(1, 0, event.Shift)
	case terminal.KeyHome:
		a.moveLineEdge(false, event.Shift)
	case terminal.KeyEnd:
		a.moveLineEdge(true, event.Shift)
	}
	return nil
}

func (a *App) runTerminalCommand(shell *shellBuffer) error {
	commandText := strings.TrimSpace(string(shell.buffer.Slice(shell.promptStart, shell.buffer.Len())))
	a.moveTerminalCursorToEnd(shell)
	shell.buffer.Insert(shell.buffer.Len(), []byte{'\n'})
	if commandText == "" {
		a.appendTerminalPrompt(shell)
		return nil
	}
	if isTerminalExit(commandText) {
		a.closeBufferNow(shell.buffer)
		return nil
	}
	if handled, err := a.changeTerminalDirectory(shell, commandText); handled {
		if err != nil {
			shell.buffer.Insert(shell.buffer.Len(), []byte(err.Error()+"\n"))
		}
		a.appendTerminalPrompt(shell)
		return nil
	}

	shell.running = true
	a.message = "terminal: " + commandText
	ctx, cancel := context.WithCancel(context.Background())
	shell.cancel = cancel
	go func() {
		command := exec.CommandContext(ctx, shell.shellPath, "-c", commandText)
		command.Dir = shell.workingPath
		output, err := command.CombinedOutput()
		text := string(output)
		if err != nil {
			if text != "" && !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			text += fmt.Sprintf("[command exited: %v]\n", err)
		}
		a.servers <- serverEvent{terminal: shell, terminalOutput: text}
	}()
	return nil
}

func userShell() string {
	configured := strings.TrimSpace(os.Getenv("SHELL"))
	if configured != "" {
		if path, err := exec.LookPath(configured); err == nil {
			return path
		}
	}
	return "/bin/sh"
}

func isTerminalExit(command string) bool {
	fields := strings.Fields(command)
	return len(fields) > 0 && fields[0] == "exit"
}

func (a *App) finishTerminalCommand(shell *shellBuffer, output string) {
	if a.terminals[shell.buffer] != shell {
		return
	}
	if output != "" {
		shell.buffer.Insert(shell.buffer.Len(), []byte(output))
		if !strings.HasSuffix(output, "\n") {
			shell.buffer.Insert(shell.buffer.Len(), []byte{'\n'})
		}
	}
	shell.running = false
	shell.cancel = nil
	a.appendTerminalPrompt(shell)
	a.message = ""
	if a.current() == shell.buffer {
		a.ensureCursorVisible()
	}
}

func (a *App) appendTerminalPrompt(shell *shellBuffer) {
	name := filepath.Base(shell.workingPath)
	if name == "." || name == string(filepath.Separator) {
		name = shell.workingPath
	}
	prompt := name + " $ "
	shell.buffer.Insert(shell.buffer.Len(), []byte(prompt))
	shell.promptStart = shell.buffer.Len()
	a.moveTerminalCursorToEnd(shell)
}

func (a *App) moveTerminalCursorToEnd(shell *shellBuffer) {
	point := shell.buffer.Point(shell.buffer.Len())
	shell.buffer.SetCursors([]buffer.Cursor{{Anchor: point, Point: point}})
}

func (a *App) changeTerminalDirectory(shell *shellBuffer, command string) (bool, error) {
	fields := strings.Fields(command)
	if len(fields) == 0 || fields[0] != "cd" {
		return false, nil
	}
	path := ""
	if len(fields) == 1 {
		path = os.Getenv("HOME")
	} else {
		path = strings.Join(fields[1:], " ")
		if !filepath.IsAbs(path) {
			path = filepath.Join(shell.workingPath, path)
		}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return true, fmt.Errorf("cd: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return true, fmt.Errorf("cd: %w", err)
	}
	if !info.IsDir() {
		return true, fmt.Errorf("cd: %s is not a directory", absolute)
	}
	shell.workingPath = absolute
	return true, nil
}
