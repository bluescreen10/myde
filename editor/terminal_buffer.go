package editor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/terminal"
	"github.com/creack/pty"
)

type shellBuffer struct {
	buffer      *buffer.Buffer
	workingPath string
	shellPath   string
	command     *exec.Cmd
	pty         *os.File
	state       *terminalState
	running     bool
	cancel      func()
	closeOnce   sync.Once
	width       int
	height      int
}

func (a *App) openTerminal(arguments string) error {
	for index, current := range a.buffers {
		if current.terminal != nil {
			a.active = index
			a.ensureCursorVisible()
			return nil
		}
	}

	current := buffer.NewNamed("*terminal*")
	a.addBuffer(current)
	current.SetHistoryLimit(0)
	shell := &shellBuffer{buffer: current, workingPath: a.root, shellPath: userShell()}
	a.currentEditorBuffer().terminal = shell
	if err := a.startTerminalShell(shell); err != nil {
		a.removeBuffer(current)
		return err
	}
	a.topLine = 0
	return nil
}

func (a *App) startTerminalShell(shell *shellBuffer) error {
	width, height := a.terminalSize()
	command := exec.Command(shell.shellPath, "-i")
	command.Dir = shell.workingPath
	command.Env = terminalEnvironment(os.Environ())
	terminalPTY, err := pty.StartWithSize(command, &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	})
	if err != nil {
		return fmt.Errorf("start interactive shell: %w", err)
	}

	shell.command = command
	shell.pty = terminalPTY
	shell.state = newTerminalState(width, height)
	shell.running = true
	shell.width = width
	shell.height = height
	shell.cancel = func() {
		shell.closeOnce.Do(func() {
			_ = terminalPTY.Close()
			if command.Process != nil {
				_ = command.Process.Kill()
			}
		})
	}

	go a.readTerminalShell(shell, terminalPTY, command)
	return nil
}

func (a *App) readTerminalShell(shell *shellBuffer, terminalPTY *os.File, command *exec.Cmd) {
	content := make([]byte, 4096)
	var readErr error
	for {
		count, err := terminalPTY.Read(content)
		if count > 0 {
			a.servers <- serverEvent{terminal: shell, terminalOutput: string(content[:count])}
		}
		if err != nil {
			readErr = err
			break
		}
	}
	waitErr := command.Wait()
	if waitErr == nil && !errors.Is(readErr, io.EOF) {
		waitErr = readErr
	}
	a.servers <- serverEvent{terminal: shell, terminalDone: true, terminalErr: waitErr}
}

func terminalEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		if strings.HasPrefix(entry, "TERM=") || strings.HasPrefix(entry, "COLORTERM=") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "TERM=xterm-256color", "COLORTERM=truecolor")
}

func (a *App) terminalSize() (int, int) {
	width, height := a.screen.Size()
	return max(20, width), max(2, height-2)
}

func (a *App) resizeTerminalShell(shell *shellBuffer) {
	if shell == nil || shell.pty == nil || !shell.running {
		return
	}
	width, height := a.terminalSize()
	if width == shell.width && height == shell.height {
		return
	}
	if err := pty.Setsize(shell.pty, &pty.Winsize{Rows: uint16(height), Cols: uint16(width)}); err != nil {
		return
	}
	shell.width = width
	shell.height = height
	shell.state.Resize(width, height)
}

func (a *App) handleTerminalEvent(event terminal.Event, shell *shellBuffer) (bool, error) {
	if !shell.running || shell.pty == nil {
		return true, nil
	}
	key := keyName(event)
	if !terminalOwnsBinding(event) && (a.bindings[key] != "" || key == "ctrl-x") {
		return false, nil
	}
	input, ok := terminalEventInput(event)
	if !ok || len(input) == 0 {
		return true, nil
	}
	if _, err := shell.pty.Write(input); err != nil {
		return true, fmt.Errorf("terminal input: %w", err)
	}
	return true, nil
}

func terminalOwnsBinding(event terminal.Event) bool {
	return event.Control && (event.Rune == 'c' || event.Rune == 'r')
}

func terminalEventInput(event terminal.Event) ([]byte, bool) {
	switch event.Key {
	case terminal.KeyRune:
		if event.Super || event.Rune == 0 {
			return nil, false
		}
		if event.Control {
			value := event.Rune
			if value >= 'A' && value <= 'Z' {
				value += 'a' - 'A'
			}
			if value >= 'a' && value <= 'z' {
				return []byte{byte(value-'a') + 1}, true
			}
			if value == ' ' {
				return []byte{0}, true
			}
			return nil, false
		}
		input := []byte(string(event.Rune))
		if event.Alt {
			input = append([]byte{0x1b}, input...)
		}
		return input, true
	case terminal.KeyEscape:
		return []byte{0x1b}, true
	case terminal.KeyEnter:
		return []byte{'\r'}, true
	case terminal.KeyBackspace:
		return []byte{0x7f}, true
	case terminal.KeyDelete:
		return []byte("\x1b[3~"), true
	case terminal.KeyTab:
		return []byte{'\t'}, true
	case terminal.KeyUp:
		return []byte("\x1b[A"), true
	case terminal.KeyDown:
		return []byte("\x1b[B"), true
	case terminal.KeyRight:
		return []byte("\x1b[C"), true
	case terminal.KeyLeft:
		return []byte("\x1b[D"), true
	case terminal.KeyHome:
		return []byte("\x1b[H"), true
	case terminal.KeyEnd:
		return []byte("\x1b[F"), true
	case terminal.KeyPageUp:
		return []byte("\x1b[5~"), true
	case terminal.KeyPageDown:
		return []byte("\x1b[6~"), true
	default:
		return nil, false
	}
}

func (a *App) appendTerminalOutput(shell *shellBuffer, output string) {
	editorBuffer := a.editorBufferFor(shell.buffer)
	if editorBuffer == nil || editorBuffer.terminal != shell || shell.state == nil || output == "" {
		return
	}
	shell.state.Write([]byte(output))
	content, point := shell.state.Snapshot()
	replaceTerminalBuffer(shell.buffer, content)
	shell.buffer.SetCursors([]buffer.Cursor{{Anchor: point, Point: point}})
	if a.current() == shell.buffer {
		a.ensureCursorVisible()
	}
}

func replaceTerminalBuffer(current *buffer.Buffer, content []byte) {
	previous := current.Bytes()
	prefix := 0
	for prefix < len(previous) && prefix < len(content) && previous[prefix] == content[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(previous)-prefix && suffix < len(content)-prefix &&
		previous[len(previous)-1-suffix] == content[len(content)-1-suffix] {
		suffix++
	}
	current.BeginTransaction()
	current.Delete(prefix, len(previous)-suffix)
	current.Insert(prefix, content[prefix:len(content)-suffix])
	current.EndTransaction()
}

func (a *App) finishTerminalCommand(shell *shellBuffer, commandErr error) {
	editorBuffer := a.editorBufferFor(shell.buffer)
	if editorBuffer == nil || editorBuffer.terminal != shell {
		return
	}
	shell.running = false
	shell.cancel = nil
	if shell.pty != nil {
		_ = shell.pty.Close()
		shell.pty = nil
	}
	unexpected := commandErr != nil && !isExpectedTerminalExit(commandErr)
	a.closeBufferNow(shell.buffer)
	if unexpected {
		a.message = "terminal: " + commandErr.Error()
	}
}

func isExpectedTerminalExit(err error) bool {
	return errors.Is(err, io.EOF) || strings.Contains(err.Error(), "input/output error")
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
