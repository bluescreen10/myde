package editor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"time"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/protocol"
)

func (a *App) debugStart(arguments string) error {
	key := a.currentEditorBuffer().mode
	if arguments == "" {
		if mode := a.modes[key]; mode != nil && mode.debugConfiguration().Program.Command != "" {
			return a.StartDebugAdapter(key)
		}
		a.prompt("Debug adapter command", func(command string) {
			if err := a.runDebugAdapterCommand(key, command); err != nil {
				a.message = err.Error()
			}
		})
		return nil
	}
	return a.runDebugAdapterCommand(key, arguments)
}

func (a *App) runDebugAdapterCommand(mode, arguments string) error {
	command, args, err := splitCommand(arguments)
	if err != nil {
		return err
	}
	return a.RunDebugAdapter(mode, plugin.DebugAdapter{Program: plugin.Program{Command: command, Arguments: args}, Transport: plugin.DebugStandardIO})
}

func (a *App) debugRestart(arguments string) error {
	if arguments != "" {
		return a.runDebugAdapterCommand(a.currentEditorBuffer().mode, arguments)
	}
	return a.RestartDebugAdapter(a.currentEditorBuffer().mode)
}

func (a *App) startDebugAdapter(configuration plugin.DebugAdapter, mode *editingMode) error {
	command, err := exec.LookPath(configuration.Program.Command)
	if err != nil {
		return fmt.Errorf("find %s: %w", configuration.Program.Command, err)
	}
	if err := a.StopDebugAdapter(mode.Name); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	var adapter *protocol.DebugProcess
	if configuration.Transport == plugin.DebugReverseTCP {
		adapter, err = protocol.StartDebugReverse(ctx, command, configuration.Program.Arguments...)
	} else {
		adapter, err = protocol.StartDebug(ctx, command, configuration.Program.Arguments...)
	}
	if err != nil {
		cancel()
		return err
	}
	session := &debugSession{mode: mode, adapter: adapter, cancel: cancel}
	mode.debugger = session
	mode.debuggerConfig = configuration
	mode.debuggerConfig.Program.Arguments = append([]string(nil), configuration.Program.Arguments...)
	requestContext, requestCancel := context.WithTimeout(ctx, 5*time.Second)
	defer requestCancel()
	err = adapter.Request(requestContext, "initialize", map[string]any{
		"clientID": "myde", "clientName": "myde", "adapterID": command,
		"pathFormat": "path", "linesStartAt1": true, "columnsStartAt1": true,
	}, nil)
	if err != nil {
		a.clearDebugAdapter(mode)
		return err
	}
	go a.readDebugEvents(session)
	a.message = mode.Name + " debug adapter started: " + command
	return nil
}

func (a *App) debugLaunch(arguments string) error {
	mode := a.currentEditorBuffer().mode
	if a.currentDebugSession() == nil {
		return fmt.Errorf("start this mode's debug adapter first")
	}
	if arguments == "" {
		a.prompt("Program to debug", func(program string) {
			if err := a.LaunchDebugProgram(mode, program); err != nil {
				a.message = err.Error()
			}
		})
		return nil
	}
	return a.LaunchDebugProgram(mode, arguments)
}

func (a *App) debugToggleBreakpoint(arguments string) error {
	path := a.current().Path()
	if path == "" {
		return fmt.Errorf("save the buffer before adding a breakpoint")
	}
	line := a.current().Cursors()[0].Point.Line
	current := a.currentEditorBuffer()
	if current.breakpoints == nil {
		current.breakpoints = make(map[int]bool)
	}
	if current.breakpoints[line] {
		delete(current.breakpoints, line)
	} else {
		current.breakpoints[line] = true
	}
	if session := a.currentDebugSession(); session != nil {
		lines := breakpointLines(current)
		go a.sendBreakpoints(session, path, lines)
	}
	return nil
}

func (a *App) debugContinue(arguments string) error {
	return a.debugExecution("continue", map[string]any{"threadId": 1})
}

func (a *App) debugNext(arguments string) error {
	return a.debugExecution("next", map[string]any{"threadId": 1})
}

func (a *App) debugStepIn(arguments string) error {
	return a.debugExecution("stepIn", map[string]any{"threadId": 1})
}

func (a *App) debugStepOut(arguments string) error {
	return a.debugExecution("stepOut", map[string]any{"threadId": 1})
}

func (a *App) debugExecution(command string, arguments any) error {
	session := a.currentDebugSession()
	if session == nil {
		return fmt.Errorf("this mode’s debug adapter is not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return session.adapter.Request(ctx, command, arguments, nil)
}

func (a *App) debugDisconnect(arguments string) error {
	if arguments == "" {
		arguments = a.currentEditorBuffer().mode
	}
	return a.StopDebugAdapter(arguments)
}

func (a *App) readDebugEvents(session *debugSession) {
	adapter := session.adapter
	defer func() { a.servers <- serverEvent{debugStopped: session} }()
	for event := range adapter.Events() {
		message := session.mode.Name + " debug: " + event.Event
		if event.Event == "initialized" {
			a.servers <- serverEvent{debugSource: session, debugReady: session}
		}
		if event.Event == "output" {
			var body struct {
				Output string `json:"output"`
			}
			if json.Unmarshal(event.Body, &body) == nil && body.Output != "" {
				message = session.mode.Name + " debug: " + body.Output
			}
		}
		a.servers <- serverEvent{debugSource: session, message: message}
		if event.Event == "terminated" {
			a.servers <- serverEvent{debugStopped: session}
			return
		}
	}
}

func (a *App) startDebugConfiguration(session *debugSession) {
	adapter := session.adapter
	breakpoints := make(map[string][]int, len(a.buffers))
	for _, current := range a.buffers {
		if current.mode == modeKey(session.mode.Name) && current.text.Path() != "" && len(current.breakpoints) > 0 {
			breakpoints[current.text.Path()] = breakpointLines(current)
		}
	}
	go func() {
		for path, lines := range breakpoints {
			a.sendBreakpoints(session, path, lines)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := adapter.Request(ctx, "configurationDone", map[string]any{}, nil); err != nil {
			a.servers <- serverEvent{debugSource: session, message: "debug configuration: " + err.Error()}
		}
	}()
}

func breakpointLines(current *editorBuffer) []int {
	lines := make([]int, 0, len(current.breakpoints))
	for line := range current.breakpoints {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	return lines
}

func (a *App) sendBreakpoints(session *debugSession, path string, lines []int) {
	adapter := session.adapter
	breakpoints := make([]map[string]int, 0, len(lines))
	for _, line := range lines {
		breakpoints = append(breakpoints, map[string]int{"line": line + 1})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := adapter.Request(ctx, "setBreakpoints", map[string]any{
		"source":      map[string]string{"path": path},
		"breakpoints": breakpoints,
	}, nil)
	if err != nil {
		a.servers <- serverEvent{debugSource: session, message: "set breakpoints: " + err.Error()}
	}
}
