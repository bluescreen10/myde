//go:build windows

package editor

import (
	"fmt"
	"os/exec"
	"strings"
)

func writeSystemClipboard(text string) error {
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"[Console]::In.ReadToEnd() | Set-Clipboard")
	command.Stdin = strings.NewReader(text)
	if err := command.Run(); err != nil {
		return fmt.Errorf("write Windows clipboard: %w", err)
	}
	return nil
}

func readSystemClipboard() (string, error) {
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"Get-Clipboard -Raw").Output()
	if err != nil {
		return "", fmt.Errorf("read Windows clipboard: %w", err)
	}
	return string(output), nil
}
