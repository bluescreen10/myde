package protocol_test

import (
	"bytes"
	"testing"

	"github.com/bluescreen10/myde/protocol"
)

func TestStreamRoundTrip(t *testing.T) {
	var data bytes.Buffer
	writer := protocol.NewStream(nil, &data)
	if err := writer.Write(map[string]any{"jsonrpc": "2.0", "id": 7, "result": "ok"}); err != nil {
		t.Fatal(err)
	}

	reader := protocol.NewStream(&data, nil)
	message, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(message.ID), "7"; got != want {
		t.Fatalf("ID = %s, want %s", got, want)
	}
	if got, want := string(message.Result), `"ok"`; got != want {
		t.Fatalf("Result = %s, want %s", got, want)
	}
}
