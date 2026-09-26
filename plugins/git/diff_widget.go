package git

import (
	"strings"

	"github.com/bluescreen10/myde/ui"
)

type diffWidget struct {
	title   string
	content []byte
}

type diffLineKind uint8

const (
	diffLinePlain diffLineKind = iota
	diffLineMeta
	diffLineHunk
	diffLineAdded
	diffLineRemoved
)

type diffLine struct {
	text        string
	kind        diffLineKind
	changeStart int
	changeEnd   int
}

func newDiffWidget(title string, content []byte) ui.Widget {
	return diffWidget{title: title, content: append([]byte(nil), content...)}
}

func (widget diffWidget) RenderWidget() ui.WidgetContent {
	content := ui.WidgetContent{Title: widget.title}
	for _, line := range parseDiffLines(widget.content) {
		content.Lines = append(content.Lines, richDiffLine(line))
	}
	return content
}

func parseDiffLines(content []byte) []diffLine {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	raw := strings.Split(text, "\n")
	lines := make([]diffLine, len(raw))
	for index, text := range raw {
		lines[index] = diffLine{text: text}
		switch {
		case strings.HasPrefix(text, "diff "), strings.HasPrefix(text, "index "),
			strings.HasPrefix(text, "--- "), strings.HasPrefix(text, "+++ "),
			strings.HasPrefix(text, "new file "), strings.HasPrefix(text, "deleted file "),
			strings.HasPrefix(text, "rename from "), strings.HasPrefix(text, "rename to "):
			lines[index].kind = diffLineMeta
		case strings.HasPrefix(text, "@@"):
			lines[index].kind = diffLineHunk
		case strings.HasPrefix(text, "+"):
			lines[index].kind = diffLineAdded
		case strings.HasPrefix(text, "-"):
			lines[index].kind = diffLineRemoved
		}
	}
	markChangedDiffCharacters(lines)
	return lines
}

func markChangedDiffCharacters(lines []diffLine) {
	for index := 0; index < len(lines); {
		if lines[index].kind != diffLineRemoved {
			index++
			continue
		}
		removedStart := index
		for index < len(lines) && lines[index].kind == diffLineRemoved {
			index++
		}
		addedStart := index
		for index < len(lines) && lines[index].kind == diffLineAdded {
			index++
		}
		pairs := min(addedStart-removedStart, index-addedStart)
		for offset := 0; offset < pairs; offset++ {
			removed := &lines[removedStart+offset]
			added := &lines[addedStart+offset]
			removedStartColumn, removedEndColumn, addedStartColumn, addedEndColumn :=
				changedDiffRuneRanges(strings.TrimPrefix(removed.text, "-"), strings.TrimPrefix(added.text, "+"))
			removed.changeStart = removedStartColumn + 1
			removed.changeEnd = removedEndColumn + 1
			added.changeStart = addedStartColumn + 1
			added.changeEnd = addedEndColumn + 1
		}
	}
}

func changedDiffRuneRanges(before, after string) (int, int, int, int) {
	left := []rune(before)
	right := []rune(after)
	prefix := 0
	for prefix < len(left) && prefix < len(right) && left[prefix] == right[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(left)-prefix && suffix < len(right)-prefix &&
		left[len(left)-1-suffix] == right[len(right)-1-suffix] {
		suffix++
	}
	return prefix, len(left) - suffix, prefix, len(right) - suffix
}

func richDiffLine(line diffLine) ui.RichTextLine {
	rich := ui.RichTextLine{}
	changed := ui.WidgetStyle{}
	switch line.kind {
	case diffLineMeta:
		rich.Style.Foreground = ui.ToneMuted
	case diffLineHunk:
		rich.Style.Foreground = ui.ToneAccent
		rich.Style.Background = ui.ToneAccent
		rich.Style.BackgroundIntensity = 14
	case diffLineAdded:
		rich.Style.Background = ui.ToneSuccess
		rich.Style.BackgroundIntensity = 22
		changed.Background = ui.ToneSuccess
		changed.BackgroundIntensity = 44
	case diffLineRemoved:
		rich.Style.Background = ui.ToneDanger
		rich.Style.BackgroundIntensity = 22
		changed.Background = ui.ToneDanger
		changed.BackgroundIntensity = 44
	}

	values := []rune(line.text)
	if line.changeStart >= line.changeEnd || line.changeStart < 0 || line.changeEnd > len(values) {
		rich.Spans = []ui.RichTextSpan{{Text: line.text}}
		return rich
	}
	if line.changeStart > 0 {
		rich.Spans = append(rich.Spans, ui.RichTextSpan{Text: string(values[:line.changeStart])})
	}
	rich.Spans = append(rich.Spans, ui.RichTextSpan{
		Text: string(values[line.changeStart:line.changeEnd]), Style: changed,
	})
	if line.changeEnd < len(values) {
		rich.Spans = append(rich.Spans, ui.RichTextSpan{Text: string(values[line.changeEnd:])})
	}
	return rich
}
