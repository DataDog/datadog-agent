// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (r *Runner) builtinWc(args []string) error {
	showLines, showWords, showBytes := false, false, false
	var files []string
	endOfFlags := false

	for _, a := range args {
		if endOfFlags || !strings.HasPrefix(a, "-") || a == "-" {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}
		for _, c := range a[1:] {
			switch c {
			case 'l':
				showLines = true
			case 'w':
				showWords = true
			case 'c':
				showBytes = true
			default:
				return fmt.Errorf("flag \"-%c\" is not allowed for command \"wc\"", c)
			}
		}
	}

	if !showLines && !showWords && !showBytes {
		showLines = true
		showWords = true
		showBytes = true
	}

	type wcCounts struct {
		lines, words, byteCount int
	}

	count := func(data []byte) wcCounts {
		return wcCounts{
			lines:     bytes.Count(data, []byte{'\n'}),
			words:     len(bytes.Fields(data)),
			byteCount: len(data),
		}
	}

	printCounts := func(c wcCounts, name string) {
		var parts []string
		if showLines {
			parts = append(parts, fmt.Sprintf("%d", c.lines))
		}
		if showWords {
			parts = append(parts, fmt.Sprintf("%d", c.words))
		}
		if showBytes {
			parts = append(parts, fmt.Sprintf("%d", c.byteCount))
		}
		out := strings.Join(parts, " ")
		if name != "" {
			out += " " + name
		}
		fmt.Fprintln(r.stdout, out)
	}

	if len(files) == 0 {
		data, err := io.ReadAll(r.stdin)
		if err != nil {
			fmt.Fprintf(r.stderr, "wc: read error: %v\n", err)
			r.exitCode = 1
			return nil
		}
		printCounts(count(data), "")
		r.exitCode = 0
		return nil
	}

	var total wcCounts
	hasError := false
	for _, f := range files {
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.dir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(r.stderr, "wc: %s: No such file or directory\n", f)
			hasError = true
			continue
		}
		c := count(data)
		total.lines += c.lines
		total.words += c.words
		total.byteCount += c.byteCount
		printCounts(c, f)
	}
	if len(files) > 1 {
		printCounts(total, "total")
	}
	if hasError {
		r.exitCode = 1
	} else {
		r.exitCode = 0
	}
	return nil
}
