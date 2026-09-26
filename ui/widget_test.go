package ui

import "testing"

var (
	_ Widget = TextWidget{}
	_ Widget = WidgetContent{}
)

func TestTextWidgetProducesPlainRichText(t *testing.T) {
	content := (TextWidget{
		Title:   "Output",
		Content: []byte("first\r\nsecond\n"),
	}).RenderWidget()
	if content.Title != "Output" || len(content.Lines) != 2 {
		t.Fatalf("content = %+v", content)
	}
	if got := content.Lines[0].Spans[0].Text; got != "first" {
		t.Fatalf("first line = %q", got)
	}
	if got := content.Lines[1].Spans[0].Text; got != "second" {
		t.Fatalf("second line = %q", got)
	}
}
