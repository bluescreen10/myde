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
	if arguments == "" {
		mode := a.modeForBuffer(a.current())
		if mode.DebugAdapter.Program.Command != "" {
			return a.startDebugAdapter(mode.DebugAdapter, a.currentEditorBuffer().mode)
		}
		a.prompt("Debug adapter command", func(command string) {
			if err := a.debugStart(command); err != nil {
				a.message = err.Error()
			}
		})
		return nil
	}
	command, commandArguments, err := splitCommand(arguments)
	if err != nil {
		return err
	}
	return a.startDebugAdapter(plugin.DebugAdapter{
		Program: plugin.Program{Command: command, Arguments: commandArguments},
	}, a.currentEditorBuffer().mode)
}

func (a *App) startDebugAdapter(configuration plugin.DebugAdapter, mode string) error {
	command, err := exec.LookPath(configuration.Program.Command)
	if err != nil {
		return fmt.Errorf("find %s: %w", configuration.Program.Command, err)
	}
	if a.dapCancel != nil {
		a.dapCancel()
	}
	a.dap = nil
	a.dapMode = ""
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
	a.dap = adapter
	a.dapCancel = cancel
	a.dapMode = mode
	requestContext, requestCancel := context.WithTimeout(ctx, 5*time.Second)
	defer requestCancel()
	err = adapter.Request(requestContext, "initialize", map[string]any{
		"clientID": "myde", "clientName": "myde", "adapterID": command,
		"pathFormat": "path", "linesStartAt1": true, "columnsStartAt1": true,
	}, nil)
	if err != nil {
		cancel()
		a.dap = nil
		return err
	}
	go a.readDebugEvents(adapter)
	a.message = "debug adapter started: " + command
	return nil
}

func (a *App) debugLaunch(arguments string) error {
	if a.dap == nil {
		return fmt.Errorf("start a debug adapter first")
	}
	if arguments == "" {
		a.prompt("Program to debug", func(program string) {
			if err := a.debugLaunch(program); err != nil {
				a.message = err.Error()
			}
		})
		return nil
	}
	adapter := a.dap
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := adapter.Request(ctx, "launch", map[string]any{
			"request": "launch", "program": arguments, "cwd": a.root,
		}, nil)
		if err != nil {
			a.servers <- serverEvent{message: "debug launch: " + err.Error()}
		}
	}()
	a.message = "debug launch requested"
	return nil
}

func (a *App) debugToggleBreakpoint(arguments string) error {
	path := a.current().Path()
	if path == "" {
		return fmt.Errorf("save the buffer before adding a breakpoint")
	}
	line := a.current().Cursors()[0].Point.Line
	current := a.currentEditorBuffer()
	if current.breakpoints[line] {
		delete(current.breakpoints, line)
	} else {
		current.breakpoints[line] = true
	}
	if a.dap != nil {
		lines := breakpointLines(current)
		go a.sendBreakpoints(a.dap, path, lines)
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
	if a.dap == nil {
		return fmt.Errorf("debug adapter is not running")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.dap.Request(ctx, command, arguments, nil)
}

func (a *App) debugDisconnect(arguments string) error {
	if a.dap == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := a.dap.Request(ctx, "disconnect", map[string]any{"terminateDebuggee": true}, nil)
	if a.dapCancel != nil {
		a.dapCancel()
	}
	a.dap = nil
	a.dapMode = ""
	return err
}

func (a *App) readDebugEvents(adapter *protocol.DebugProcess) {
	for event := range adapter.Events() {
		message := "debug: " + event.Event
		if event.Event == "initialized" {
			a.servers <- serverEvent{debugReady: adapter}
		}
		if event.Event == "output" {
			var body struct {
				Output string `json:"output"`
			}
			if json.Unmarshal(event.Body, &body) == nil && body.Output != "" {
				message = "debug: " + body.Output
			}
		}
		a.servers <- serverEvent{message: message}
	}
}

func (a *App) startDebugConfiguration(adapter *protocol.DebugProcess) {
	breakpoints := make(map[string][]int, len(a.buffers))
	for _, current := range a.buffers {
		if current.text.Path() != "" && len(current.breakpoints) > 0 {
			breakpoints[current.text.Path()] = breakpointLines(current)
		}
	}
	go func() {
		for path, lines := range breakpoints {
			a.sendBreakpoints(adapter, path, lines)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := adapter.Request(ctx, "configurationDone", map[string]any{}, nil); err != nil {
			a.servers <- serverEvent{message: "debug configuration: " + err.Error()}
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

func (a *App) sendBreakpoints(adapter *protocol.DebugProcess, path string, lines []int) {
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
		a.servers <- serverEvent{message: "set breakpoints: " + err.Error()}
	}
}
