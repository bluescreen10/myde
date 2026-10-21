package protocol_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bluescreen10/myde/protocol"
)

func TestProtocolHelper(t *testing.T) {
	mode := os.Args[len(os.Args)-1]
	if mode != "rpc-exchange" && mode != "rpc-disconnect" {
		return
	}
	stream := protocol.NewStream(os.Stdin, os.Stdout)
	request, err := stream.Read()
	if err != nil {
		os.Exit(2)
	}
	if mode == "rpc-disconnect" {
		os.Exit(0)
	}
	if stream.Write(map[string]any{"jsonrpc": "2.0", "id": "server-1", "method": "workspace/configuration", "params": map[string]any{"items": []any{map[string]any{}}}}) != nil {
		os.Exit(3)
	}
	response, err := stream.Read()
	if err != nil || string(response.ID) != `"server-1"` || string(response.Result) != `[null]` {
		os.Exit(4)
	}
	if stream.Write(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": "ok"}) != nil {
		os.Exit(5)
	}
	// Wait for cancellation so the client can consume its response before EOF.
	_, _ = stream.Read()
	os.Exit(0)
}

func TestProcessAnswersServerRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process, err := protocol.StartWithOptions(ctx, executable, protocol.ProcessOptions{OnRequest: func(method string, params json.RawMessage) (any, error) {
		if method != "workspace/configuration" {
			return nil, fmt.Errorf("unexpected %s", method)
		}
		return []any{nil}, nil
	}}, "-test.run=^TestProtocolHelper$", "--", "rpc-exchange")
	if err != nil {
		t.Fatal(err)
	}
	var result string
	if err := process.Request(ctx, "initialize", nil, &result); err != nil {
		t.Fatal(err)
	}
	if result != "ok" {
		t.Fatalf("result=%q", result)
	}
}

func TestProcessReportsDisconnectWithoutRequestTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process, err := protocol.Start(ctx, executable, "-test.run=^TestProtocolHelper$", "--", "rpc-disconnect")
	if err != nil {
		t.Fatal(err)
	}
	err = process.Request(ctx, "initialize", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "disconnected") {
		t.Fatalf("request error=%v", err)
	}
	if process.Running() {
		t.Fatal("disconnected process reports running")
	}
}
