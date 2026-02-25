// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const grepMaxRecursionDepth = 10

// builtinGrep implements the POSIX grep command.
// Safety: -r/-R recursion is capped at depth 10.
func (r *Runner) builtinGrep(ctx context.Context, args []string) exitStatus {
	var exit exitStatus

	var (
		ignoreCase   bool
		invertMatch  bool
		countOnly    bool
		filesMatch   bool // -l
		filesNoMatch bool // -L
		lineNumbers  bool
		forceFilename bool // -H
		noFilename    bool // -h
		recursive    bool
		wordMatch    bool
		lineMatch    bool // -x
		quiet        bool
		suppress     bool // -s
		maxCount     int
		fixedStr     bool // -F
		patterns     []string
	)

	fp := flagParser{remaining: args}
	for fp.more() {
		switch flag := fp.flag(); flag {
		case "-i":
			ignoreCase = true
		case "-v":
			invertMatch = true
		case "-c":
			countOnly = true
		case "-l":
			filesMatch = true
		case "-L":
			filesNoMatch = true
		case "-n":
			lineNumbers = true
		case "-H":
			forceFilename = true
			noFilename = false
		case "-h":
			noFilename = true
			forceFilename = false
		case "-r", "-R":
			recursive = true
		case "-e":
			v := fp.value()
			if v == "" {
				r.errf("grep: option requires an argument -- 'e'\n")
				exit.code = 2
				return exit
			}
			patterns = append(patterns, v)
		case "-w":
			wordMatch = true
		case "-x":
			lineMatch = true
		case "-q":
			quiet = true
		case "-s":
			suppress = true
		case "-m":
			v := fp.value()
			if v == "" {
				r.errf("grep: option requires an argument -- 'm'\n")
				exit.code = 2
				return exit
			}
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil || n <= 0 {
				r.errf("grep: invalid max count: %q\n", v)
				exit.code = 2
				return exit
			}
			maxCount = int(n)
		case "-E":
			// Extended regex — Go's regexp is already ERE-compatible.
		case "-F":
			fixedStr = true
		default:
			r.errf("grep: invalid option %q\n", flag)
			exit.code = 2
			return exit
		}
	}

	remaining := fp.args()

	if len(patterns) == 0 {
		if len(remaining) == 0 {
			r.errf("usage: grep [OPTION]... PATTERN [FILE]...\n")
			exit.code = 2
			return exit
		}
		patterns = append(patterns, remaining[0])
		remaining = remaining[1:]
	}

	var regexps []*regexp.Regexp
	for _, pat := range patterns {
		if fixedStr {
			pat = regexp.QuoteMeta(pat)
		}
		if wordMatch {
			pat = `\b` + pat + `\b`
		}
		if lineMatch {
			pat = "^" + pat + "$"
		}
		if ignoreCase {
			pat = "(?i)" + pat
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			r.errf("grep: invalid pattern: %v\n", err)
			exit.code = 2
			return exit
		}
		regexps = append(regexps, re)
	}

	paths := remaining
	showFilename := forceFilename || (len(paths) > 1 && !noFilename) || (recursive && !noFilename)

	anyMatch := false

	if len(paths) == 0 {
		if r.stdin == nil {
			exit.code = 1
			return exit
		}
		matched := grepStream(r, r.stdin, "", regexps, invertMatch, countOnly, filesMatch, filesNoMatch,
			lineNumbers, false, quiet, maxCount)
		if matched {
			anyMatch = true
		}
	} else {
		for _, p := range paths {
			absP := r.absPath(p)
			info, err := r.stat(ctx, absP)
			if err != nil {
				if !suppress {
					r.errf("grep: %s: No such file or directory\n", p)
				}
				exit.code = 2
				continue
			}

			if info.IsDir() {
				if !recursive {
					r.errf("grep: %s: Is a directory\n", p)
					exit.code = 2
					continue
				}
				matched := grepRecursive(ctx, r, p, absP, 0, regexps, invertMatch, countOnly,
					filesMatch, filesNoMatch, lineNumbers, showFilename, quiet, suppress, maxCount)
				if matched {
					anyMatch = true
				}
				continue
			}

			f, err := r.open(ctx, absP, os.O_RDONLY, 0, !suppress)
			if err != nil {
				exit.code = 2
				continue
			}
			fname := ""
			if showFilename {
				fname = p
			}
			matched := grepStream(r, f, fname, regexps, invertMatch, countOnly, filesMatch, filesNoMatch,
				lineNumbers, showFilename, quiet, maxCount)
			f.Close()
			if matched {
				anyMatch = true
			}
		}
	}

	if !anyMatch {
		exit.code = 1
	}
	return exit
}

func grepRecursive(ctx context.Context, r *Runner, displayPath, absPath string, depth int,
	regexps []*regexp.Regexp, invertMatch, countOnly, filesMatch, filesNoMatch, lineNumbers, showFilename, quiet, suppress bool, maxCount int) bool {

	if depth >= grepMaxRecursionDepth {
		r.errf("grep: warning: recursive search depth exceeded at '%s' (max %d)\n", displayPath, grepMaxRecursionDepth)
		return false
	}

	entries, err := r.readDirHandler(r.handlerCtx(ctx, handlerKindReadDir, todoPos), absPath)
	if err != nil {
		if !suppress {
			r.errf("grep: %s: %v\n", displayPath, err)
		}
		return false
	}

	anyMatch := false
	for _, de := range entries {
		select {
		case <-ctx.Done():
			return anyMatch
		default:
		}

		name := de.Name()
		entryDisplay := filepath.Join(displayPath, name)
		entryAbs := filepath.Join(absPath, name)

		info, err := r.stat(ctx, entryAbs)
		if err != nil {
			if !suppress {
				r.errf("grep: %s: %v\n", entryDisplay, err)
			}
			continue
		}

		if info.IsDir() {
			if grepRecursive(ctx, r, entryDisplay, entryAbs, depth+1, regexps, invertMatch,
				countOnly, filesMatch, filesNoMatch, lineNumbers, showFilename, quiet, suppress, maxCount) {
				anyMatch = true
			}
			continue
		}

		if !info.Mode().IsRegular() {
			continue
		}

		f, err := r.open(ctx, entryAbs, os.O_RDONLY, 0, !suppress)
		if err != nil {
			continue
		}
		fname := ""
		if showFilename {
			fname = entryDisplay
		}
		if grepStream(r, f, fname, regexps, invertMatch, countOnly, filesMatch, filesNoMatch,
			lineNumbers, showFilename, quiet, maxCount) {
			anyMatch = true
		}
		f.Close()
	}

	return anyMatch
}

func grepStream(r *Runner, reader io.Reader, filename string,
	regexps []*regexp.Regexp, invertMatch, countOnly, filesMatch, filesNoMatch, lineNumbers, showFilename, quiet bool, maxCount int) bool {

	scanner := bufio.NewScanner(reader)
	lineNo := 0
	matchCount := 0

	for scanner.Scan() {
		lineNo++
		line := scanner.Text()

		matched := false
		for _, re := range regexps {
			if re.MatchString(line) {
				matched = true
				break
			}
		}

		if invertMatch {
			matched = !matched
		}

		if !matched {
			continue
		}

		matchCount++

		if quiet {
			return true
		}

		if filesMatch {
			if filename != "" {
				r.outf("%s\n", filename)
			}
			return true
		}

		if !countOnly && !filesNoMatch {
			var prefix string
			if showFilename && filename != "" {
				prefix = filename + ":"
			}
			if lineNumbers {
				r.outf("%s%d:%s\n", prefix, lineNo, line)
			} else {
				r.outf("%s%s\n", prefix, line)
			}
		}

		if maxCount > 0 && matchCount >= maxCount {
			break
		}
	}

	if filesNoMatch && matchCount == 0 && filename != "" {
		r.outf("%s\n", filename)
		return false
	}

	if countOnly {
		if showFilename && filename != "" {
			r.outf("%s:%d\n", filename, matchCount)
		} else {
			r.outf("%d\n", matchCount)
		}
	}

	return matchCount > 0
}
