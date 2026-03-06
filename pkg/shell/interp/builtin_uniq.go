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
	"strings"
)

func (r *Runner) builtinUniq(args []string) error {
	showCount := false
	dupsOnly := false
	ignoreCase := false
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
			case 'c':
				showCount = true
			case 'd':
				dupsOnly = true
			case 'i':
				ignoreCase = true
			default:
				return fmt.Errorf("flag \"-%c\" is not allowed for command \"uniq\"", c)
			}
		}
	}

	var reader io.Reader
	if len(files) > 0 {
		path := files[0]
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.dir, path)
		}
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(r.stderr, "uniq: %s: No such file or directory\n", files[0])
			r.exitCode = 1
			return nil
		}
		defer f.Close()
		reader = f
	} else {
		reader = r.stdin
	}

	scanner := bufio.NewScanner(reader)
	var prevLine string
	prevSet := false
	count := 0

	outputLine := func(line string, cnt int) {
		if dupsOnly && cnt < 2 {
			return
		}
		if showCount {
			fmt.Fprintf(r.stdout, "%d %s\n", cnt, line)
		} else {
			fmt.Fprintln(r.stdout, line)
		}
	}

	equal := func(a, b string) bool {
		if ignoreCase {
			return strings.EqualFold(a, b)
		}
		return a == b
	}

	for scanner.Scan() {
		line := scanner.Text()
		if !prevSet {
			prevLine = line
			prevSet = true
			count = 1
			continue
		}
		if equal(line, prevLine) {
			count++
		} else {
			outputLine(prevLine, count)
			prevLine = line
			count = 1
		}
	}

	if prevSet {
		outputLine(prevLine, count)
	}

	r.exitCode = 0
	return nil
}
