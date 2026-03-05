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
	"strconv"
	"strings"
)

func (r *Runner) builtinTail(args []string) error {
	lineCount := 10
	byteCount := -1
	fromLine := -1 // for +N syntax: output starting from line N (1-indexed)
	var files []string
	endOfFlags := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		if endOfFlags || (!strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "+")) || a == "-" {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}

		// Handle +N syntax (output starting from line N).
		if strings.HasPrefix(a, "+") {
			n, err := strconv.Atoi(a[1:])
			if err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number: %q\n", a)
				r.exitCode = 1
				return nil
			}
			fromLine = n
			lineCount = -1
			byteCount = -1
			continue
		}

		switch {
		case a == "-n" || a == "--lines":
			i++
			if i >= len(args) {
				fmt.Fprintf(r.stderr, "tail: option requires an argument -- 'n'\n")
				r.exitCode = 1
				return nil
			}
			val := args[i]
			if strings.HasPrefix(val, "+") {
				n, err := strconv.Atoi(val[1:])
				if err != nil {
					fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", val)
					r.exitCode = 1
					return nil
				}
				fromLine = n
				lineCount = -1
				byteCount = -1
			} else {
				n, err := strconv.Atoi(val)
				if err != nil {
					fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", val)
					r.exitCode = 1
					return nil
				}
				lineCount = n
				fromLine = -1
				byteCount = -1
			}
		case strings.HasPrefix(a, "-n"):
			val := a[2:]
			if strings.HasPrefix(val, "+") {
				n, err := strconv.Atoi(val[1:])
				if err != nil {
					fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", val)
					r.exitCode = 1
					return nil
				}
				fromLine = n
				lineCount = -1
				byteCount = -1
			} else {
				n, err := strconv.Atoi(val)
				if err != nil {
					fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", val)
					r.exitCode = 1
					return nil
				}
				lineCount = n
				fromLine = -1
				byteCount = -1
			}
		case strings.HasPrefix(a, "--lines="):
			val := a[len("--lines="):]
			n, err := strconv.Atoi(val)
			if err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", val)
				r.exitCode = 1
				return nil
			}
			lineCount = n
			fromLine = -1
			byteCount = -1
		case a == "-c" || a == "--bytes":
			i++
			if i >= len(args) {
				fmt.Fprintf(r.stderr, "tail: option requires an argument -- 'c'\n")
				r.exitCode = 1
				return nil
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", args[i])
				r.exitCode = 1
				return nil
			}
			byteCount = n
			lineCount = -1
			fromLine = -1
		case strings.HasPrefix(a, "-c"):
			n, err := strconv.Atoi(a[2:])
			if err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", a[2:])
				r.exitCode = 1
				return nil
			}
			byteCount = n
			lineCount = -1
			fromLine = -1
		case strings.HasPrefix(a, "--bytes="):
			n, err := strconv.Atoi(a[len("--bytes="):])
			if err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", a[len("--bytes="):])
				r.exitCode = 1
				return nil
			}
			byteCount = n
			lineCount = -1
			fromLine = -1
		default:
			return fmt.Errorf("flag %q is not allowed for command \"tail\"", a)
		}
	}

	tailOutput := func(data []byte, name string, showHeader bool) {
		if showHeader {
			fmt.Fprintf(r.stdout, "==> %s <==\n", name)
		}
		if byteCount >= 0 {
			if byteCount >= len(data) {
				r.stdout.Write(data)
			} else {
				r.stdout.Write(data[len(data)-byteCount:])
			}
		} else if fromLine >= 0 {
			lines := bytes.Split(data, []byte{'\n'})
			start := fromLine - 1 // convert 1-indexed to 0-indexed
			if start < 0 {
				start = 0
			}
			if start < len(lines) {
				for i := start; i < len(lines); i++ {
					if i == len(lines)-1 && len(lines[i]) == 0 {
						break // skip trailing empty line from split
					}
					r.stdout.Write(lines[i])
					r.stdout.Write([]byte{'\n'})
				}
			}
		} else {
			lines := splitKeepNewlines(data)
			start := len(lines) - lineCount
			if start < 0 {
				start = 0
			}
			for i := start; i < len(lines); i++ {
				r.stdout.Write([]byte(lines[i]))
			}
		}
	}

	if len(files) == 0 {
		data, err := io.ReadAll(r.stdin)
		if err != nil {
			fmt.Fprintf(r.stderr, "tail: error reading stdin: %v\n", err)
			r.exitCode = 1
			return nil
		}
		tailOutput(data, "", false)
		r.exitCode = 0
		return nil
	}

	showHeader := len(files) > 1
	hasError := false
	for idx, f := range files {
		if idx > 0 && showHeader {
			fmt.Fprintln(r.stdout)
		}
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.dir, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(r.stderr, "tail: cannot open %q for reading: No such file or directory\n", f)
			hasError = true
			continue
		}
		tailOutput(data, f, showHeader)
	}
	if hasError {
		r.exitCode = 1
	} else {
		r.exitCode = 0
	}
	return nil
}

// splitKeepNewlines splits data into lines, keeping the newline at the end of each line.
func splitKeepNewlines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var lines []string
	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx == -1 {
			lines = append(lines, string(data))
			break
		}
		lines = append(lines, string(data[:idx+1]))
		data = data[idx+1:]
	}
	return lines
}
