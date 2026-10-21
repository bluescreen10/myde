package editor_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bluescreen10/myde/terminal"
)

func TestPersistentTerminalShellHasPTYAndKeepsState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a Unix PTY")
	}
	t.Setenv("SHELL", "/bin/sh")
	h := newHarness(t, "")
	h.exec("terminal.open")
	h.text("MYDE_PERSIST=works\nif [ -t 0 ]; then MYDE_TTY=yes; else MYDE_TTY=no; fi\nprintf 'MYDE_MARK:%s:%s\\n' \"$MYDE_PERSIST\" \"$MYDE_TTY\"\n")
	h.wait("persistent shell state", func() bool { return strings.Contains(h.content(), "MYDE_MARK:works:yes") })
	h.text("exit\n")
	h.wait("shell exit", func() bool { return !strings.Contains(h.screen.row(0), "*terminal*") })
}

func TestPersistentZshUsesNativeHistory(t *testing.T) {
	path, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is unavailable")
	}
	t.Setenv("SHELL", path)
	config := t.TempDir()
	if err := os.WriteFile(filepath.Join(config, ".zshrc"), []byte("PROMPT='MYDE_PROMPT> '\nRPS1=\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZDOTDIR", config)
	t.Setenv("MYDE_NATIVE_MARK", "native-history-marker")
	h := newHarness(t, "")
	h.exec("terminal.open")
	h.wait("prompt", func() bool { return strings.Contains(h.content(), "MYDE_PROMPT> ") })
	h.text("printf '%s\\n' \"$MYDE_NATIVE_MARK\"\n")
	h.wait("first history command", func() bool { return strings.Count(h.content(), "native-history-marker") == 1 })
	h.key(terminal.Event{Key: terminal.KeyUp})
	h.key(terminal.Event{Key: terminal.KeyEnter})
	h.wait("history replay", func() bool { return strings.Count(h.content(), "native-history-marker") == 2 })
	h.key(terminal.Event{Key: terminal.KeyRune, Rune: 'r', Control: true})
	h.text("printf\n")
	h.wait("native reverse search", func() bool { return strings.Count(h.content(), "native-history-marker") == 3 })
	promptCount := strings.Count(h.content(), "MYDE_PROMPT> ")
	h.text("/bin/sh -c 'printf \"SLEEP_%s\\n\" ready; exec sleep 10'\n")
	h.wait("sleep ready", func() bool { return strings.Contains(h.content(), "SLEEP_ready") })
	time.Sleep(100 * time.Millisecond)
	h.key(terminal.Event{Key: terminal.KeyRune, Rune: 'c', Control: true})
	h.wait("interrupted prompt", func() bool { return strings.Count(h.content(), "MYDE_PROMPT> ") > promptCount })
	h.text("printf 'INTERRUPT_%s\\n' ok\n")
	h.wait("native interrupt", func() bool { return strings.Contains(h.content(), "INTERRUPT_ok") })
}

func scriptedTerminal(t *testing.T, script string) *harness {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires /bin/sh")
	}
	path := filepath.Join(t.TempDir(), "shell")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\nread MYDE_WAIT\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", path)
	h := newHarness(t, "")
	h.exec("terminal.open")
	return h
}
