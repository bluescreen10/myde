// Command myde starts the myde terminal editor.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/bluescreen10/myde/editor"
	"github.com/bluescreen10/myde/terminal"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "myde:", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	rootFlag := flag.String("root", "", "workspace root")
	versionFlag := flag.Bool("version", false, "print version")
	flag.Parse()
	if *versionFlag {
		fmt.Println("myde", version)
		return nil
	}

	root, paths, err := resolvePaths(*rootFlag, flag.Args())
	if err != nil {
		return err
	}
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return fmt.Errorf("standard input and output must be terminals")
	}

	session, err := terminal.Open(os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	defer func() {
		if err := session.Close(); err != nil && runErr == nil {
			runErr = err
		}
	}()
	stopSignals := restoreTerminalOnSignal(session)
	defer stopSignals()

	app, err := editor.New(root, paths, session, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	return app.Run()
}

func restoreTerminalOnSignal(session *terminal.Session) func() {
	signals := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(signals, terminationSignals()...)
	go func() {
		select {
		case <-signals:
			_ = session.Close()
			os.Exit(1)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(signals)
		close(done)
	}
}

func resolvePaths(root string, arguments []string) (string, []string, error) {
	if root == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return "", nil, fmt.Errorf("read working directory: %w", err)
		}
		root = workingDirectory
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", nil, fmt.Errorf("resolve workspace: %w", err)
	}
	paths := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		info, statErr := os.Stat(argument)
		if statErr == nil && info.IsDir() {
			absoluteRoot, err = filepath.Abs(argument)
			if err != nil {
				return "", nil, fmt.Errorf("resolve workspace: %w", err)
			}
			continue
		}
		paths = append(paths, argument)
	}
	return absoluteRoot, paths, nil
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
