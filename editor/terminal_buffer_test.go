package editor

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/terminal"
)

func TestPersistentTerminalShellHasPTYAndKeepsState(t *testing.T) {
	shellPath, err := exec.LookPath("/bin/sh")
	if err != nil {
		t.Skip("/bin/sh is not available")
	}
	current := buffer.NewNamed("*terminal*")
	shell := &shellBuffer{buffer: current, workingPath: t.TempDir(), shellPath: shellPath}
	app := &App{
		buffers:  []*editorBuffer{{text: current, terminal: shell}},
		screen:   terminal.NewScreen(io.Discard, 80, 24),
		servers:  make(chan serverEvent, 64),
		bindings: make(map[string]string),
	}
	if err := app.startTerminalShell(shell); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if shell.cancel != nil {
			shell.cancel()
		}
	})

	_, err = shell.pty.Write([]byte("MYDE_PERSIST=works\r" +
		"if [ -t 0 ]; then MYDE_TTY=yes; else MYDE_TTY=no; fi\r" +
		"printf 'MYDE_MARK:%s:%s\\n' \"$MYDE_PERSIST\" \"$MYDE_TTY\"\r"))
	if err != nil {
		t.Fatal(err)
	}
	waitForTerminalText(t, app, shell, "MYDE_MARK:works:yes", 3*time.Second)

	_, _ = shell.pty.Write([]byte("exit\r"))
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event := <-app.servers:
			if event.terminalDone {
				return
			}
			app.handleServerEvent(event)
		case <-deadline:
			t.Fatal("interactive shell did not exit")
		}
	}
}

func TestPersistentZshUsesNativeHistory(t *testing.T) {
	shellPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not available")
	}
	configDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDirectory, ".zshrc"), []byte("PROMPT='MYDE_PROMPT> '\nRPS1=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZDOTDIR", configDirectory)
	t.Setenv("MYDE_NATIVE_HISTORY_MARKER", "native-history-from-zsh")
	t.Setenv("MYDE_INTERRUPT_MARKER", "native-interrupt-from-zsh")
	t.Setenv("MYDE_SLEEP_READY_MARKER", "native-sleep-is-running")

	current := buffer.NewNamed("*terminal*")
	shell := &shellBuffer{buffer: current, workingPath: t.TempDir(), shellPath: shellPath}
	app := &App{
		buffers:  []*editorBuffer{{text: current, terminal: shell}},
		screen:   terminal.NewScreen(io.Discard, 80, 24),
		servers:  make(chan serverEvent, 64),
		bindings: make(map[string]string),
	}
	if err := app.startTerminalShell(shell); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if shell.cancel != nil {
			shell.cancel()
		}
	})

	waitForTerminalTextCount(t, app, shell, "MYDE_PROMPT> ", 1, 3*time.Second)
	writeTerminalInput(t, shell, []byte("printf '%s\\n' \"$MYDE_NATIVE_HISTORY_MARKER\"\r"))
	waitForTerminalTextCount(t, app, shell, "native-history-from-zsh", 1, 3*time.Second)
	waitForTerminalTextCount(t, app, shell, "MYDE_PROMPT> ", 2, 3*time.Second)

	up, _ := terminalEventInput(terminal.Event{Key: terminal.KeyUp})
	enter, _ := terminalEventInput(terminal.Event{Key: terminal.KeyEnter})
	writeTerminalInput(t, shell, append(up, enter...))
	waitForTerminalTextCount(t, app, shell, "native-history-from-zsh", 2, 3*time.Second)
	waitForTerminalTextCount(t, app, shell, "MYDE_PROMPT> ", 3, 3*time.Second)

	reverseSearch, _ := terminalEventInput(terminal.Event{Key: terminal.KeyRune, Rune: 'r', Control: true})
	writeTerminalInput(t, shell, append(append(reverseSearch, []byte("printf")...), enter...))
	waitForTerminalTextCount(t, app, shell, "native-history-from-zsh", 3, 3*time.Second)
	waitForTerminalTextCount(t, app, shell, "MYDE_PROMPT> ", 4, 3*time.Second)

	writeTerminalInput(t, shell, []byte("/bin/sh -c 'printf \"%s\\n\" \"$MYDE_SLEEP_READY_MARKER\"; exec sleep 10'\r"))
	waitForTerminalTextCount(t, app, shell, "native-sleep-is-running", 1, 3*time.Second)
	time.Sleep(100 * time.Millisecond)
	interrupt, _ := terminalEventInput(terminal.Event{Key: terminal.KeyRune, Rune: 'c', Control: true})
	writeTerminalInput(t, shell, interrupt)
	waitForTerminalTextCount(t, app, shell, "MYDE_PROMPT> ", 5, 3*time.Second)
	writeTerminalInput(t, shell, []byte("printf '%s\\n' \"$MYDE_INTERRUPT_MARKER\"\r"))
	waitForTerminalTextCount(t, app, shell, "native-interrupt-from-zsh", 1, 3*time.Second)
}

func writeTerminalInput(t *testing.T, shell *shellBuffer, input []byte) {
	t.Helper()
	if _, err := shell.pty.Write(input); err != nil {
		t.Fatal(err)
	}
}

func waitForTerminalText(t *testing.T, app *App, shell *shellBuffer, wanted string, timeout time.Duration) {
	waitForTerminalTextCount(t, app, shell, wanted, 1, timeout)
}

func waitForTerminalTextCount(t *testing.T, app *App, shell *shellBuffer, wanted string, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case event := <-app.servers:
			if event.terminalDone {
				t.Fatalf("interactive shell exited before %q: %v", wanted, event.terminalErr)
			}
			app.handleServerEvent(event)
			if strings.Count(string(shell.buffer.Bytes()), wanted) >= count {
				return
			}
		case <-deadline:
			t.Fatalf("terminal did not render %d copies of %q; content = %q", count, wanted, shell.buffer.Bytes())
		}
	}
}

func TestTerminalInputSequences(t *testing.T) {
	tests := []struct {
		event terminal.Event
		want  string
	}{
		{event: terminal.Event{Key: terminal.KeyRune, Rune: 'c', Control: true}, want: "\x03"},
		{event: terminal.Event{Key: terminal.KeyRune, Rune: 'r', Control: true}, want: "\x12"},
		{event: terminal.Event{Key: terminal.KeyUp}, want: "\x1b[A"},
		{event: terminal.Event{Key: terminal.KeyEnter}, want: "\r"},
		{event: terminal.Event{Key: terminal.KeyRune, Rune: 'x', Alt: true}, want: "\x1bx"},
	}
	for _, test := range tests {
		got, ok := terminalEventInput(test.event)
		if !ok || string(got) != test.want {
			t.Errorf("terminalEventInput(%+v) = %q, %v; want %q", test.event, got, ok, test.want)
		}
	}
}

func TestTerminalOnlyOverridesNativeControlBindings(t *testing.T) {
	if !terminalOwnsBinding(terminal.Event{Key: terminal.KeyRune, Rune: 'c', Control: true}) {
		t.Fatal("Ctrl-C must be delivered to the shell")
	}
	if !terminalOwnsBinding(terminal.Event{Key: terminal.KeyRune, Rune: 'r', Control: true}) {
		t.Fatal("Ctrl-R must be delivered to the shell")
	}
	if terminalOwnsBinding(terminal.Event{Key: terminal.KeyUp, Control: true}) {
		t.Fatal("a configured Ctrl-Up editor binding must take precedence")
	}
}
