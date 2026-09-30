// Package golang provides Go language support for myde.
package golang

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bluescreen10/myde/plugin"
)

type goPlugin struct {
	host plugin.Host
}

const (
	formatOnSaveSetting  = "go-format-on-save"
	importsOnSaveSetting = "go-imports-on-save"
)

// New returns the Go plugin.
func New() plugin.Plugin {
	return &goPlugin{}
}

func (g *goPlugin) Name() string {
	return "go"
}

func (g *goPlugin) Load(host plugin.Host) error {
	g.host = host
	if err := host.RegisterMode(plugin.Mode{
		Name:       "Go",
		Extensions: []string{".go"},
		Syntax:     "go",
		LanguageID: "go",
		LanguageServer: plugin.Program{
			Command: "gopls",
		},
		DebugAdapter: plugin.DebugAdapter{
			Program: plugin.Program{
				Command:   "dlv",
				Arguments: []string{"dap", "--client-addr={address}"},
			},
			Transport: plugin.DebugReverseTCP,
		},
	}); err != nil {
		return err
	}
	commands := []struct {
		name    string
		handler plugin.Command
	}{
		{name: "go.fmt", handler: g.format},
		{name: "go.vet", handler: g.vet},
		{name: "go.build", handler: g.build},
		{name: "go.test", handler: g.test},
	}
	for _, command := range commands {
		if err := host.RegisterCommand(command.name, command.handler); err != nil {
			return err
		}
	}
	for _, formatter := range []struct {
		command string
		setting string
	}{
		{command: "goimports", setting: importsOnSaveSetting},
		{command: "gofmt", setting: formatOnSaveSetting},
	} {
		if _, err := exec.LookPath(formatter.command); err != nil {
			continue
		}
		command := formatter.command
		setting := formatter.setting
		if err := host.Subscribe(plugin.EventFileBeforeSave, func(event plugin.Event) error {
			return g.formatBeforeSave(event, command, setting)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (g *goPlugin) format(arguments string) error {
	if strings.TrimSpace(arguments) != "" {
		return fmt.Errorf("go.fmt does not accept arguments")
	}
	document := g.host.CurrentDocument()
	if document.ReadOnly {
		return fmt.Errorf("current buffer is read-only")
	}
	formatted, err := formatContent("gofmt", document.Content, document.Path)
	if err != nil {
		return err
	}
	if err := g.host.ReplaceCurrentDocument(formatted); err != nil {
		return err
	}
	g.host.SetMessage("formatted current buffer")
	return nil
}

func (g *goPlugin) formatBeforeSave(event plugin.Event, command, setting string) error {
	if filepath.Ext(event.Value) != ".go" {
		return nil
	}
	enabled, err := g.settingEnabled(setting, true)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	document := g.host.CurrentDocument()
	if document.ReadOnly {
		return nil
	}
	formatted, err := formatContent(command, document.Content, event.Value)
	if err != nil {
		return err
	}
	return g.host.ReplaceCurrentDocument(formatted)
}

func (g *goPlugin) settingEnabled(name string, fallback bool) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(g.host.Setting(name)))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", name)
	}
}

func formatContent(name string, content []byte, path string) ([]byte, error) {
	command := exec.Command(name)
	if path != "" {
		command.Dir = filepath.Dir(path)
	}
	command.Stdin = bytes.NewReader(content)
	formatted, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(formatted))
		if detail == "" {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return nil, fmt.Errorf("%s: %w: %s", name, err, detail)
	}
	return formatted, nil
}

func (g *goPlugin) vet(arguments string) error {
	return g.runPackageCommand("vet", arguments)
}

func (g *goPlugin) build(arguments string) error {
	return g.runPackageCommand("build", arguments)
}

func (g *goPlugin) test(arguments string) error {
	return g.runPackageCommand("test", arguments)
}

func (g *goPlugin) runPackageCommand(action, arguments string) error {
	if strings.TrimSpace(arguments) != "" {
		return fmt.Errorf("go.%s does not accept arguments", action)
	}
	document := g.host.CurrentDocument()
	if document.Path == "" {
		return fmt.Errorf("save the buffer before running go.%s", action)
	}
	if document.Dirty {
		return fmt.Errorf("save the buffer before running go.%s", action)
	}
	commandArguments := []string{action}
	if action == "build" {
		commandArguments = append(commandArguments, "-o", os.DevNull)
	}
	commandArguments = append(commandArguments, ".")
	command := exec.Command("go", commandArguments...)
	command.Dir = filepath.Dir(document.Path)
	output, err := command.CombinedOutput()
	if len(output) > 0 {
		name := fmt.Sprintf("*go %s: %s*.log", action, filepath.Base(command.Dir))
		g.host.OpenReadOnlyBuffer(name, output)
	}
	if err != nil {
		return fmt.Errorf("go %s failed: %w", action, err)
	}
	g.host.SetMessage("go " + action + " succeeded")
	return nil
}
