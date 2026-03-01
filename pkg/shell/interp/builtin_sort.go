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
	"sort"
	"strconv"
	"strings"
	"unicode"
)

func (r *Runner) builtinSort(args []string) error {
	reverse := false
	numeric := false
	unique := false
	foldCase := false
	humanNumeric := false
	keyField := 0 // 0 means sort whole line, >0 means 1-indexed field
	separator := ""
	var files []string
	endOfFlags := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		if endOfFlags || !strings.HasPrefix(a, "-") || a == "-" {
			files = append(files, a)
			continue
		}
		if a == "--" {
			endOfFlags = true
			continue
		}

		// Handle flags that take values: -k, -t
		switch {
		case a == "-k":
			i++
			if i >= len(args) {
				fmt.Fprintf(r.stderr, "sort: option requires an argument -- 'k'\n")
				r.exitCode = 1
				return nil
			}
			// Parse key spec: N or N,M (we use just the start field)
			keySpec := args[i]
			if idx := strings.IndexByte(keySpec, ','); idx >= 0 {
				keySpec = keySpec[:idx]
			}
			n, err := strconv.Atoi(keySpec)
			if err != nil || n < 1 {
				fmt.Fprintf(r.stderr, "sort: invalid key specification: %q\n", args[i])
				r.exitCode = 1
				return nil
			}
			keyField = n
			continue
		case strings.HasPrefix(a, "-k"):
			keySpec := a[2:]
			if idx := strings.IndexByte(keySpec, ','); idx >= 0 {
				keySpec = keySpec[:idx]
			}
			n, err := strconv.Atoi(keySpec)
			if err != nil || n < 1 {
				fmt.Fprintf(r.stderr, "sort: invalid key specification: %q\n", a[2:])
				r.exitCode = 1
				return nil
			}
			keyField = n
			continue
		case a == "-t":
			i++
			if i >= len(args) {
				fmt.Fprintf(r.stderr, "sort: option requires an argument -- 't'\n")
				r.exitCode = 1
				return nil
			}
			separator = args[i]
			continue
		case strings.HasPrefix(a, "-t"):
			separator = a[2:]
			continue
		}

		// Handle boolean flags (may be combined: -rn).
		for j := 1; j < len(a); j++ {
			switch a[j] {
			case 'r':
				reverse = true
			case 'n':
				numeric = true
			case 'u':
				unique = true
			case 'f':
				foldCase = true
			case 'h':
				humanNumeric = true
			default:
				return fmt.Errorf("flag \"-%c\" is not allowed for command \"sort\"", a[j])
			}
		}
	}

	// Read input lines.
	var lines []string
	if len(files) == 0 {
		lines = readLines(r.stdin)
	} else {
		for _, f := range files {
			path := f
			if !filepath.IsAbs(path) {
				path = filepath.Join(r.dir, path)
			}
			file, err := os.Open(path)
			if err != nil {
				fmt.Fprintf(r.stderr, "sort: cannot read: %s: No such file or directory\n", f)
				r.exitCode = 1
				return nil
			}
			lines = append(lines, readLines(file)...)
			file.Close()
		}
	}

	// Extract sort key from a line.
	getKey := func(line string) string {
		if keyField == 0 {
			return line
		}
		sep := separator
		var fields []string
		if sep != "" {
			fields = strings.Split(line, sep)
		} else {
			fields = strings.Fields(line)
		}
		idx := keyField - 1
		if idx >= len(fields) {
			return ""
		}
		return fields[idx]
	}

	// Sort.
	sort.SliceStable(lines, func(i, j int) bool {
		a, b := getKey(lines[i]), getKey(lines[j])
		if foldCase {
			a = strings.ToLower(a)
			b = strings.ToLower(b)
		}
		var less bool
		if humanNumeric {
			less = parseHumanNumeric(a) < parseHumanNumeric(b)
		} else if numeric {
			less = parseNumeric(a) < parseNumeric(b)
		} else {
			less = a < b
		}
		if reverse {
			return !less
		}
		return less
	})

	// Deduplicate if -u.
	if unique {
		deduped := lines[:0]
		for i, line := range lines {
			if i == 0 {
				deduped = append(deduped, line)
				continue
			}
			prev := getKey(deduped[len(deduped)-1])
			cur := getKey(line)
			if foldCase {
				prev = strings.ToLower(prev)
				cur = strings.ToLower(cur)
			}
			if prev != cur {
				deduped = append(deduped, line)
			}
		}
		lines = deduped
	}

	// Output.
	for _, line := range lines {
		fmt.Fprintln(r.stdout, line)
	}

	r.exitCode = 0
	return nil
}

func readLines(reader io.Reader) []string {
	var lines []string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func parseNumeric(s string) float64 {
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseHumanNumeric(s string) float64 {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0
	}
	multiplier := 1.0
	last := s[len(s)-1]
	switch unicode.ToUpper(rune(last)) {
	case 'K':
		multiplier = 1024
		s = s[:len(s)-1]
	case 'M':
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	case 'G':
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case 'T':
		multiplier = 1024 * 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f * multiplier
}
