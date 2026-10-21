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
		DebugAdapter: goDebugAdapter(),
	}); err != nil {
		return err
	}
	commands := []struct {
		name    string
		handler plugin.Command
	}{
		{name: "go.fmt", handler: g.format},
		{name: "go.debug.start", handler: g.modeCommand("go.debug.start", host.StartDebugAdapter)},
		{name: "go.debug.stop", handler: g.modeCommand("go.debug.stop", host.StopDebugAdapter)},
		{name: "go.debug.restart", handler: g.modeCommand("go.debug.restart", host.RestartDebugAdapter)},
		{name: "go.debug.run", handler: g.runDebugger},
		{name: "go.lsp.run", handler: g.runLanguageServer},
		{name: "go.lsp.start", handler: g.modeCommand("go.lsp.start", host.StartLanguageServer)},
		{name: "go.lsp.stop", handler: g.modeCommand("go.lsp.stop", host.StopLanguageServer)},
		{name: "go.lsp.restart", handler: g.modeCommand("go.lsp.restart", host.RestartLanguageServer)},
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

func (g *goPlugin) modeCommand(name string, command func(string) error) plugin.Command {
	return func(arguments string) error {
		if strings.TrimSpace(arguments) != "" {
			return fmt.Errorf("%s does not accept arguments", name)
		}
		return command("Go")
	}
}

func (g *goPlugin) runLanguageServer(arguments string) error {
	fields := strings.Fields(arguments)
	program := plugin.Program{Command: "gopls"}
	if len(fields) > 0 {
		program.Command = fields[0]
		program.Arguments = fields[1:]
	}
	return g.host.RunLanguageServer("Go", program)
}

func goDebugAdapter() plugin.DebugAdapter {
	return plugin.DebugAdapter{Program: plugin.Program{Command: "dlv", Arguments: []string{"dap", "--client-addr={address}"}}, Transport: plugin.DebugReverseTCP}
}

func (g *goPlugin) runDebugger(arguments string) error {
	if strings.TrimSpace(arguments) == "" {
		g.host.Prompt("Go program to debug", g.runDebugger)
		return nil
	}
	if err := g.host.StartDebugAdapter("Go"); err != nil {
		return err
	}
	return g.host.LaunchDebugProgram("Go", arguments)
}
