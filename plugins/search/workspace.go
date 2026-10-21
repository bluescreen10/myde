package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const resultLimit = 1000

type result struct {
	path             string
	line, byteColumn int
	text             string
}

func searchWorkspace(ctx context.Context, root string, files []string, query string) ([]result, error) {
	command := exec.CommandContext(ctx, "rg", "--json", "--fixed-strings", "--smart-case", "--color", "never", "--", query, ".")
	command.Dir = root
	output, err := command.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err == nil {
		return parseRipgrepSearchResults(output), nil
	}
	var commandError *exec.Error
	if errors.As(err, &commandError) && errors.Is(commandError.Err, exec.ErrNotFound) {
		return searchWorkspaceFiles(ctx, root, files, query)
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return nil, nil
	}
	return nil, fmt.Errorf("search workspace: %w", err)
}

type ripgrepJSONEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
		} `json:"submatches"`
	} `json:"data"`
}

func parseRipgrepSearchResults(output []byte) []result {
	results := make([]result, 0)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() && len(results) < resultLimit {
		var event ripgrepJSONEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Type != "match" {
			continue
		}
		column := 0
		if len(event.Data.Submatches) > 0 {
			column = event.Data.Submatches[0].Start
		}
		path := strings.TrimPrefix(filepath.ToSlash(event.Data.Path.Text), "./")
		results = append(results, result{
			path: filepath.FromSlash(path), line: max(0, event.Data.LineNumber-1),
			byteColumn: column, text: strings.TrimRight(event.Data.Lines.Text, "\r\n"),
		})
	}
	return results
}

func searchWorkspaceFiles(ctx context.Context, root string, files []string, query string) ([]result, error) {
	results := make([]result, 0)
	for _, relative := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		contents, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil || bytes.IndexByte(contents, 0) >= 0 {
			continue
		}
		results = append(results, searchWorkspaceDocument(relative, contents, query)...)
		if len(results) >= resultLimit {
			return results[:resultLimit], nil
		}
	}
	return results, nil
}

func searchWorkspaceDocument(path string, contents []byte, query string) []result {
	results := make([]result, 0)
	caseSensitive := strings.ToLower(query) != query
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(needle)
	}
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		text := scanner.Text()
		haystack := text
		if !caseSensitive {
			haystack = strings.ToLower(haystack)
		}
		if column := strings.Index(haystack, needle); column >= 0 {
			results = append(results, result{path: path, line: line, byteColumn: column, text: text})
			if len(results) == resultLimit {
				break
			}
		}
		line++
	}
	return results
}
