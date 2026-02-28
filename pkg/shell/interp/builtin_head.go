// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (r *Runner) builtinHead(args []string) error {
	lineCount := 10
	byteCount := -1
	var files []string
	endOfFlags := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		if endOfFlags || (!strings.HasPrefix(a, "-") || a == "-") {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}

		switch {
		case a == "-n" || a == "--lines":
			i++
			if i >= len(args) {
				fmt.Fprintf(r.stderr, "head: option requires an argument -- 'n'\n")
				r.exitCode = 1
				return nil
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(r.stderr, "head: invalid number of lines: %q\n", args[i])
				r.exitCode = 1
				return nil
			}
			lineCount = n
			byteCount = -1
		case strings.HasPrefix(a, "-n"):
			n, err := strconv.Atoi(a[2:])
			if err != nil {
				fmt.Fprintf(r.stderr, "head: invalid number of lines: %q\n", a[2:])
				r.exitCode = 1
				return nil
			}
			lineCount = n
			byteCount = -1
		case strings.HasPrefix(a, "--lines="):
			n, err := strconv.Atoi(a[len("--lines="):])
			if err != nil {
				fmt.Fprintf(r.stderr, "head: invalid number of lines: %q\n", a[len("--lines="):])
				r.exitCode = 1
				return nil
			}
			lineCount = n
			byteCount = -1
		case a == "-c" || a == "--bytes":
			i++
			if i >= len(args) {
				fmt.Fprintf(r.stderr, "head: option requires an argument -- 'c'\n")
				r.exitCode = 1
				return nil
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(r.stderr, "head: invalid number of bytes: %q\n", args[i])
				r.exitCode = 1
				return nil
			}
			byteCount = n
			lineCount = -1
		case strings.HasPrefix(a, "-c"):
			n, err := strconv.Atoi(a[2:])
			if err != nil {
				fmt.Fprintf(r.stderr, "head: invalid number of bytes: %q\n", a[2:])
				r.exitCode = 1
				return nil
			}
			byteCount = n
			lineCount = -1
		case strings.HasPrefix(a, "--bytes="):
			n, err := strconv.Atoi(a[len("--bytes="):])
			if err != nil {
				fmt.Fprintf(r.stderr, "head: invalid number of bytes: %q\n", a[len("--bytes="):])
				r.exitCode = 1
				return nil
			}
			byteCount = n
			lineCount = -1
		default:
			return fmt.Errorf("flag %q is not allowed for command \"head\"", a)
		}
	}

	headOutput := func(reader io.Reader, name string, showHeader bool) error {
		if showHeader {
			fmt.Fprintf(r.stdout, "==> %s <==\n", name)
		}
		if byteCount >= 0 {
			buf := make([]byte, byteCount)
			n, _ := io.ReadFull(reader, buf)
			if n > 0 {
				r.stdout.Write(buf[:n])
			}
		} else {
			scanner := bufio.NewScanner(reader)
			count := 0
			for scanner.Scan() {
				if count >= lineCount {
					break
				}
				fmt.Fprintln(r.stdout, scanner.Text())
				count++
			}
		}
		return nil
	}

	if len(files) == 0 {
		headOutput(r.stdin, "", false)
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
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(r.stderr, "head: cannot open %q for reading: No such file or directory\n", f)
			hasError = true
			continue
		}
		headOutput(file, f, showHeader)
		file.Close()
	}
	if hasError {
		r.exitCode = 1
	} else {
		r.exitCode = 0
	}
	return nil
}
