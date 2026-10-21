package editor

import (
	"context"
	"fmt"
	"time"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/protocol"
)

type debugSession struct {
	mode    *editingMode
	adapter *protocol.DebugProcess
	cancel  context.CancelFunc
}

func (m *editingMode) debugConfiguration() plugin.DebugAdapter {
	if m.debuggerConfig.Program.Command != "" {
		return m.debuggerConfig
	}
	return m.DebugAdapter
}

func (a *App) currentDebugSession() *debugSession {
	mode := a.modes[a.currentEditorBuffer().mode]
	if mode == nil || mode.debugger == nil || !mode.debugger.adapter.Running() {
		return nil
	}
	return mode.debugger
}

// StartDebugAdapter reuses or starts the adapter registered for this mode.
func (a *App) StartDebugAdapter(name string) error {
	mode, err := a.registeredMode(name)
	if err != nil {
		return err
	}
	if mode.debugger != nil && mode.debugger.adapter.Running() {
		return nil
	}
	configuration := mode.debugConfiguration()
	if configuration.Program.Command == "" {
		return fmt.Errorf("mode %s has no debug adapter", mode.Name)
	}
	return a.RunDebugAdapter(name, configuration)
}

// RunDebugAdapter replaces only this mode's debug adapter.
func (a *App) RunDebugAdapter(name string, configuration plugin.DebugAdapter) error {
	mode, err := a.registeredMode(name)
	if err != nil {
		return err
	}
	if configuration.Program.Command == "" {
		return fmt.Errorf("debug adapter command cannot be empty")
	}
	if configuration.Transport == "" {
		configuration.Transport = plugin.DebugStandardIO
	}
	if configuration.Transport != plugin.DebugStandardIO && configuration.Transport != plugin.DebugReverseTCP {
		return fmt.Errorf("unsupported debug transport %q", configuration.Transport)
	}
	if configuration.Transport == plugin.DebugReverseTCP && !containsAddressPlaceholder(configuration.Program.Arguments) {
		return fmt.Errorf("reverse TCP adapter must include {address} in its arguments")
	}
	return a.startDebugAdapter(configuration, mode)
}

// StopDebugAdapter disconnects this mode's debuggee and stops its adapter.
func (a *App) StopDebugAdapter(name string) error {
	mode, err := a.registeredMode(name)
	if err != nil {
		return err
	}
	session := mode.debugger
	if session == nil {
		return nil
	}
	var disconnectErr error
	if session.adapter.Running() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		disconnectErr = session.adapter.Request(ctx, "disconnect", map[string]any{"terminateDebuggee": true}, nil)
		cancel()
	}
	a.clearDebugAdapter(mode)
	a.message = mode.Name + " debug adapter stopped"
	return disconnectErr
}

// RestartDebugAdapter starts a fresh adapter for one mode.
func (a *App) RestartDebugAdapter(name string) error {
	mode, err := a.registeredMode(name)
	if err != nil {
		return err
	}
	return a.RunDebugAdapter(name, mode.debugConfiguration())
}

func (a *App) clearDebugAdapter(mode *editingMode) {
	session := mode.debugger
	mode.debugger = nil
	if session == nil {
		return
	}
	session.cancel()
	_ = session.adapter.Close()
}

// LaunchDebugProgram launches a program in this mode's running debugger.
func (a *App) LaunchDebugProgram(name, program string) error {
	mode, err := a.registeredMode(name)
	if err != nil {
		return err
	}
	session := mode.debugger
	if session == nil || !session.adapter.Running() {
		return fmt.Errorf("%s debug adapter is not running", mode.Name)
	}
	if program == "" {
		return fmt.Errorf("program to debug cannot be empty")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := session.adapter.Request(ctx, "launch", map[string]any{"request": "launch", "program": program, "cwd": a.root}, nil)
		if err != nil {
			a.servers <- serverEvent{debugSource: session, message: mode.Name + " debug launch: " + err.Error()}
		}
	}()
	a.message = mode.Name + " debug launch requested"
	return nil
}
