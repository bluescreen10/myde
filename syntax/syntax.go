// Package syntax provides incremental, line-oriented source highlighting.
package syntax

import (
	"path/filepath"
	"strings"
	"unicode"
)

// Kind classifies highlighted source text.
type Kind uint8

const (
	Plain Kind = iota
	Comment
	Keyword
	String
	Number
	Type
)

// Span assigns a syntax kind to a half-open rune range.
type Span struct {
	Start int
	End   int
	Kind  Kind
}

type lineState struct {
	inBlockComment bool
}

type lineResult struct {
	text  string
	start lineState
	end   lineState
	spans []Span
}

// Highlighter caches parser state per line and reparses only invalidated tails.
type Highlighter struct {
	language string
	lines    []lineResult
}

// New returns a highlighter selected from a file name.
func New(path string) *Highlighter {
	return &Highlighter{language: languageForPath(path)}
}

// Invalidate discards parser state at and after line.
func (h *Highlighter) Invalidate(line int) {
	if line < 0 {
		line = 0
	}
	if line < len(h.lines) {
		h.lines = h.lines[:line]
	}
}

// Highlight returns syntax spans for a line. Calls should normally be in line order.
func (h *Highlighter) Highlight(line int, text string) []Span {
	if line < len(h.lines) && h.lines[line].text == text {
		return h.lines[line].spans
	}
	if line < len(h.lines) {
		h.lines = h.lines[:line]
	}
	for len(h.lines) <= line {
		start := lineState{}
		if len(h.lines) > 0 {
			start = h.lines[len(h.lines)-1].end
		}
		current := ""
		if len(h.lines) == line {
			current = text
		}
		result := h.parseLine(current, start)
		h.lines = append(h.lines, result)
	}
	return h.lines[line].spans
}

func (h *Highlighter) parseLine(text string, start lineState) lineResult {
	runes := []rune(text)
	result := lineResult{text: text, start: start, end: start}
	for i := 0; i < len(runes); {
		if result.end.inBlockComment {
			end := findPair(runes, i, '*', '/')
			if end < 0 {
				result.spans = append(result.spans, Span{Start: i, End: len(runes), Kind: Comment})
				break
			}
			result.spans = append(result.spans, Span{Start: i, End: end + 2, Kind: Comment})
			result.end.inBlockComment = false
			i = end + 2
			continue
		}
		if startsPair(runes, i, '/', '/') || (h.language == "shell" && runes[i] == '#') {
			result.spans = append(result.spans, Span{Start: i, End: len(runes), Kind: Comment})
			break
		}
		if startsPair(runes, i, '/', '*') {
			end := findPair(runes, i+2, '*', '/')
			if end < 0 {
				result.spans = append(result.spans, Span{Start: i, End: len(runes), Kind: Comment})
				result.end.inBlockComment = true
				break
			}
			result.spans = append(result.spans, Span{Start: i, End: end + 2, Kind: Comment})
			i = end + 2
			continue
		}
		if runes[i] == '"' || runes[i] == '\'' || runes[i] == '`' {
			end := stringEnd(runes, i, runes[i])
			result.spans = append(result.spans, Span{Start: i, End: end, Kind: String})
			i = end
			continue
		}
		if unicode.IsDigit(runes[i]) {
			end := i + 1
			for end < len(runes) && (unicode.IsDigit(runes[end]) || strings.ContainsRune("._xabcdefABCDEF", runes[end])) {
				end++
			}
			result.spans = append(result.spans, Span{Start: i, End: end, Kind: Number})
			i = end
			continue
		}
		if unicode.IsLetter(runes[i]) || runes[i] == '_' {
			end := i + 1
			for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end]) || runes[end] == '_') {
				end++
			}
			word := string(runes[i:end])
			if isKeyword(h.language, word) {
				result.spans = append(result.spans, Span{Start: i, End: end, Kind: Keyword})
			} else if len(word) > 0 && unicode.IsUpper([]rune(word)[0]) {
				result.spans = append(result.spans, Span{Start: i, End: end, Kind: Type})
			}
			i = end
			continue
		}
		i++
	}
	return result
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".js", ".jsx", ".ts", ".tsx":
		return "javascript"
	case ".py":
		return "python"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".c", ".h", ".cc", ".cpp", ".hpp":
		return "c"
	default:
		return "plain"
	}
}

func isKeyword(language, word string) bool {
	keywords := map[string]string{
		"go":         "break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var",
		"rust":       "as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while",
		"javascript": "async await break case catch class const continue debugger default delete do else export extends false finally for from function get if import in instanceof let new null of return set static super switch this throw true try typeof undefined var void while with yield",
		"python":     "and as assert async await break class continue def del elif else except False finally for from global if import in is lambda None nonlocal not or pass raise return True try while with yield",
		"shell":      "case do done elif else esac export fi for function if in local readonly then while",
		"c":          "auto break case char const continue default do double else enum extern float for goto if inline int long register restrict return short signed sizeof static struct switch typedef union unsigned void volatile while",
	}
	return strings.Contains(" "+keywords[language]+" ", " "+word+" ")
}

func startsPair(runes []rune, index int, first, second rune) bool {
	return index+1 < len(runes) && runes[index] == first && runes[index+1] == second
}

func findPair(runes []rune, start int, first, second rune) int {
	for i := start; i+1 < len(runes); i++ {
		if runes[i] == first && runes[i+1] == second {
			return i
		}
	}
	return -1
}

func stringEnd(runes []rune, start int, quote rune) int {
	escaped := false
	for i := start + 1; i < len(runes); i++ {
		if runes[i] == quote && !escaped {
			return i + 1
		}
		if quote != '`' && runes[i] == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}
	return len(runes)
}
