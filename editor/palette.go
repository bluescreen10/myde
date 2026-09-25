package editor

import (
	"encoding/json"
	"sort"
	"strings"
)

type paletteItem struct {
	label         string
	detail        string
	documentation string
	value         string
	kind          string
	resolve       json.RawMessage
	resolved      bool
}

type palette struct {
	title              string
	query              []rune
	items              []paletteItem
	filtered           []paletteItem
	selected           int
	onChoose           func(paletteItem)
	onSubmit           func(string)
	source             func(string) ([]paletteItem, string)
	completion         bool
	localCompletion    bool
	completionRevision uint64
}

type minibuffer struct {
	title    string
	query    []rune
	onSubmit func(string)
}

func (p *palette) update() {
	type scored struct {
		item  paletteItem
		score int
	}
	query := string(p.query)
	if p.source != nil {
		p.items, query = p.source(query)
	}
	query = strings.ToLower(query)
	results := make([]scored, 0, len(p.items))
	for _, item := range p.items {
		score, ok := fuzzyScore(strings.ToLower(item.label+" "+item.detail), query)
		if ok {
			results = append(results, scored{item: item, score: score})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	p.filtered = p.filtered[:0]
	for _, result := range results {
		p.filtered = append(p.filtered, result.item)
	}
	if p.selected >= len(p.filtered) {
		p.selected = max(0, len(p.filtered)-1)
	}
}

func fuzzyScore(text, query string) (int, bool) {
	if query == "" {
		return 0, true
	}
	score := 0
	position := 0
	previous := -2
	for _, wanted := range query {
		found := strings.IndexRune(text[position:], wanted)
		if found < 0 {
			return 0, false
		}
		absolute := position + found
		if absolute == previous+1 {
			score += 8
		} else {
			score += 2
		}
		if absolute == 0 || strings.ContainsRune(" /._-", rune(text[absolute-1])) {
			score += 5
		}
		previous = absolute
		position = absolute + 1
	}
	return score - len(text)/20, true
}
