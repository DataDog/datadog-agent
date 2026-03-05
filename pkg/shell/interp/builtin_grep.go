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
	"regexp"
	"strconv"
	"strings"
)

type grepOpts struct {
	ignoreCase    bool
	invertMatch   bool
	countOnly     bool
	filesOnly     bool
	showLineNums  bool
	recursive     bool
	patterns      []string
	wholeWord     bool
	extendedRegex bool
	fixedStrings  bool
	maxCount      int
	afterContext   int
	beforeContext  int
	context       int
	include       []string
	exclude       []string
	excludeDir    []string
}

func (r *Runner) builtinGrep(args []string) error {
	opts := grepOpts{maxCount: -1}
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
		// Long flags with values.
		case strings.HasPrefix(a, "--include="):
			opts.include = append(opts.include, a[len("--include="):])
		case a == "--include":
			i++
			if i >= len(args) {
				return fmt.Errorf("grep: option '--include' requires an argument")
			}
			opts.include = append(opts.include, args[i])
		case strings.HasPrefix(a, "--exclude="):
			opts.exclude = append(opts.exclude, a[len("--exclude="):])
		case a == "--exclude":
			i++
			if i >= len(args) {
				return fmt.Errorf("grep: option '--exclude' requires an argument")
			}
			opts.exclude = append(opts.exclude, args[i])
		case strings.HasPrefix(a, "--exclude-dir="):
			opts.excludeDir = append(opts.excludeDir, a[len("--exclude-dir="):])
		case a == "--exclude-dir":
			i++
			if i >= len(args) {
				return fmt.Errorf("grep: option '--exclude-dir' requires an argument")
			}
			opts.excludeDir = append(opts.excludeDir, args[i])

		// Flags that take a value: -e, -m, -A, -B, -C
		case a == "-e":
			i++
			if i >= len(args) {
				return fmt.Errorf("grep: option requires an argument -- 'e'")
			}
			opts.patterns = append(opts.patterns, args[i])
		case strings.HasPrefix(a, "-e"):
			opts.patterns = append(opts.patterns, a[2:])

		case a == "-m":
			i++
			if i >= len(args) {
				return fmt.Errorf("grep: option requires an argument -- 'm'")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("grep: invalid max count '%s'", args[i])
			}
			opts.maxCount = n
		case strings.HasPrefix(a, "-m"):
			n, err := strconv.Atoi(a[2:])
			if err != nil {
				return fmt.Errorf("grep: invalid max count '%s'", a[2:])
			}
			opts.maxCount = n

		case a == "-A":
			i++
			if i >= len(args) {
				return fmt.Errorf("grep: option requires an argument -- 'A'")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("grep: invalid context count '%s'", args[i])
			}
			opts.afterContext = n
		case strings.HasPrefix(a, "-A"):
			n, err := strconv.Atoi(a[2:])
			if err != nil {
				return fmt.Errorf("grep: invalid context count '%s'", a[2:])
			}
			opts.afterContext = n

		case a == "-B":
			i++
			if i >= len(args) {
				return fmt.Errorf("grep: option requires an argument -- 'B'")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("grep: invalid context count '%s'", args[i])
			}
			opts.beforeContext = n
		case strings.HasPrefix(a, "-B"):
			n, err := strconv.Atoi(a[2:])
			if err != nil {
				return fmt.Errorf("grep: invalid context count '%s'", a[2:])
			}
			opts.beforeContext = n

		case a == "-C":
			i++
			if i >= len(args) {
				return fmt.Errorf("grep: option requires an argument -- 'C'")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("grep: invalid context count '%s'", args[i])
			}
			opts.context = n
		case strings.HasPrefix(a, "-C"):
			n, err := strconv.Atoi(a[2:])
			if err != nil {
				return fmt.Errorf("grep: invalid context count '%s'", a[2:])
			}
			opts.context = n

		default:
			// Handle combined short flags (e.g., -inr).
			for j := 1; j < len(a); j++ {
				switch a[j] {
				case 'i':
					opts.ignoreCase = true
				case 'v':
					opts.invertMatch = true
				case 'c':
					opts.countOnly = true
				case 'l':
					opts.filesOnly = true
				case 'n':
					opts.showLineNums = true
				case 'r':
					opts.recursive = true
				case 'w':
					opts.wholeWord = true
				case 'E':
					opts.extendedRegex = true
				case 'F':
					opts.fixedStrings = true
				default:
					return fmt.Errorf("flag \"-%c\" is not allowed for command \"grep\"", a[j])
				}
			}
		}
	}

	// The first non-option argument is the pattern (if -e was not used).
	if len(opts.patterns) == 0 {
		if len(files) == 0 {
			fmt.Fprintf(r.stderr, "grep: missing pattern\n")
			r.exitCode = 2
			return nil
		}
		opts.patterns = append(opts.patterns, files[0])
		files = files[1:]
	}

	// If -C is set, it sets both -A and -B.
	if opts.context > 0 {
		if opts.afterContext == 0 {
			opts.afterContext = opts.context
		}
		if opts.beforeContext == 0 {
			opts.beforeContext = opts.context
		}
	}

	// Compile the regex pattern.
	re, err := compileGrepPattern(opts.patterns, opts.ignoreCase, opts.wholeWord, opts.fixedStrings)
	if err != nil {
		fmt.Fprintf(r.stderr, "grep: invalid pattern: %v\n", err)
		r.exitCode = 2
		return nil
	}

	// If recursive and no files given, search current directory.
	if opts.recursive && len(files) == 0 {
		files = []string{"."}
	}

	// Determine if we should show filenames.
	showFilenames := len(files) > 1 || opts.recursive

	totalMatches := 0

	if len(files) == 0 {
		// Read from stdin.
		matches := r.grepReader(re, r.stdin, "", false, &opts)
		totalMatches += matches
	} else {
		for _, f := range files {
			path := f
			if !filepath.IsAbs(path) {
				path = filepath.Join(r.dir, path)
			}

			info, err := os.Stat(path)
			if err != nil {
				fmt.Fprintf(r.stderr, "grep: %s: No such file or directory\n", f)
				r.exitCode = 2
				continue
			}

			if info.IsDir() {
				if opts.recursive {
					matches := r.grepRecursive(re, path, f, &opts, showFilenames)
					totalMatches += matches
				} else {
					fmt.Fprintf(r.stderr, "grep: %s: Is a directory\n", f)
				}
			} else {
				file, err := os.Open(path)
				if err != nil {
					fmt.Fprintf(r.stderr, "grep: %s: %v\n", f, err)
					r.exitCode = 2
					continue
				}
				matches := r.grepReader(re, file, f, showFilenames, &opts)
				file.Close()
				totalMatches += matches
			}
		}
	}

	if totalMatches > 0 {
		r.exitCode = 0
	} else {
		r.exitCode = 1
	}
	return nil
}

func compileGrepPattern(patterns []string, ignoreCase, wholeWord, fixedStrings bool) (*regexp.Regexp, error) {
	// Build alternation of all patterns.
	var regexParts []string
	for _, p := range patterns {
		if fixedStrings {
			p = regexp.QuoteMeta(p)
		}
		if wholeWord {
			p = `\b` + p + `\b`
		}
		regexParts = append(regexParts, p)
	}

	combined := strings.Join(regexParts, "|")
	if ignoreCase {
		combined = "(?i)" + combined
	}

	return regexp.Compile(combined)
}

func (r *Runner) grepReader(re *regexp.Regexp, reader io.Reader, filename string, showFilename bool, opts *grepOpts) int {
	scanner := bufio.NewScanner(reader)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	matchCount := 0
	printedUpTo := -1 // track context line printing

	for lineIdx, line := range lines {
		matched := re.MatchString(line)
		if opts.invertMatch {
			matched = !matched
		}

		if !matched {
			continue
		}

		matchCount++

		if opts.filesOnly {
			fmt.Fprintln(r.stdout, filename)
			return matchCount
		}

		if opts.countOnly {
			if opts.maxCount >= 0 && matchCount >= opts.maxCount {
				break
			}
			continue
		}

		// Print before-context lines.
		if opts.beforeContext > 0 {
			start := lineIdx - opts.beforeContext
			if start < 0 {
				start = 0
			}
			if start <= printedUpTo {
				start = printedUpTo + 1
			}
			if start < lineIdx && start > printedUpTo+1 && printedUpTo >= 0 {
				fmt.Fprintln(r.stdout, "--")
			}
			for ci := start; ci < lineIdx; ci++ {
				r.grepPrintLine(filename, showFilename, ci+1, lines[ci], opts, '-')
			}
		} else if opts.beforeContext == 0 && opts.afterContext > 0 && lineIdx > printedUpTo+1 && printedUpTo >= 0 {
			fmt.Fprintln(r.stdout, "--")
		}

		r.grepPrintLine(filename, showFilename, lineIdx+1, line, opts, ':')
		printedUpTo = lineIdx

		// Print after-context lines.
		if opts.afterContext > 0 {
			end := lineIdx + opts.afterContext
			if end >= len(lines) {
				end = len(lines) - 1
			}
			for ci := lineIdx + 1; ci <= end; ci++ {
				r.grepPrintLine(filename, showFilename, ci+1, lines[ci], opts, '-')
				printedUpTo = ci
			}
		}

		if opts.maxCount >= 0 && matchCount >= opts.maxCount {
			break
		}
	}

	if opts.countOnly {
		if showFilename {
			fmt.Fprintf(r.stdout, "%s:%d\n", filename, matchCount)
		} else {
			fmt.Fprintf(r.stdout, "%d\n", matchCount)
		}
	}

	return matchCount
}

func (r *Runner) grepPrintLine(filename string, showFilename bool, lineNum int, line string, opts *grepOpts, separator byte) {
	var prefix string
	if showFilename {
		prefix = filename + string(separator)
	}
	if opts.showLineNums {
		prefix += fmt.Sprintf("%d%c", lineNum, separator)
	}
	fmt.Fprintf(r.stdout, "%s%s\n", prefix, line)
}

func (r *Runner) grepRecursive(re *regexp.Regexp, absPath, displayPath string, opts *grepOpts, showFilenames bool) int {
	totalMatches := 0

	filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			// Check --exclude-dir.
			for _, pattern := range opts.excludeDir {
				if matched, _ := filepath.Match(pattern, info.Name()); matched {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Check --include.
		if len(opts.include) > 0 {
			included := false
			for _, pattern := range opts.include {
				if matched, _ := filepath.Match(pattern, info.Name()); matched {
					included = true
					break
				}
			}
			if !included {
				return nil
			}
		}

		// Check --exclude.
		for _, pattern := range opts.exclude {
			if matched, _ := filepath.Match(pattern, info.Name()); matched {
				return nil
			}
		}

		relPath, _ := filepath.Rel(absPath, path)
		dispName := filepath.Join(displayPath, relPath)

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		matches := r.grepReader(re, file, dispName, showFilenames, opts)
		totalMatches += matches
		return nil
	})

	return totalMatches
}
