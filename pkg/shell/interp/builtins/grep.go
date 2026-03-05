// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

// Package builtins implements POSIX shell builtin commands.
//
// grep — print lines that match patterns
//
// Usage: grep [OPTION]... PATTERN [FILE]...
//        grep [OPTION]... -e PATTERN... [FILE]...
//
// Search for PATTERN in each FILE or standard input.
// PATTERN is, by default, an extended regular expression (ERE) using Go RE2 syntax.
// With no FILE, or when FILE is -, read standard input.
// Exit status is 0 if any line matches, 1 if no line matches, 2 if an error occurs.
//
// Supported flags:
//
//	-e PAT, --regexp=PAT         Add PAT to the list of patterns (may be repeated).
//	-i, --ignore-case            Case-insensitive matching.
//	-v, --invert-match           Print lines that do NOT match.
//	-n, --line-number            Prefix each output line with its 1-based line number.
//	-c, --count                  Print only the count of matching lines per file.
//	-l, --files-with-matches     Print only filenames that contain at least one match.
//	-L, --files-without-match    Print only filenames that contain no match.
//	-q, --quiet, --silent        No output; exit code only (0 = match found).
//	-s, --no-messages            Suppress error messages for missing/unreadable files.
//	-w, --word-regexp            Match only whole words (surrounded by word boundaries).
//	-x, --line-regexp            Match only whole lines (anchored ^ and $).
//	-F, --fixed-strings          Treat PATTERN as a literal string, not a regex.
//	-E, --extended-regexp        Use extended regular expression syntax (default in Go; accepted for compatibility).
//	-H, --with-filename          Always print filename prefix for each match.
//	    --no-filename             Never print filename prefix (-h reserved for --help).
//	-m N, --max-count=N          Stop reading each file after N matching lines.
//	-o, --only-matching          Print only the matched (non-empty) portion of each line.
//	-A N, --after-context=N      Print N lines of trailing context after each match.
//	-B N, --before-context=N     Print N lines of leading context before each match.
//	-C N, --context=N            Print N lines of output context (sets both -A and -B).
//	-h, --help                   Print this help message and exit.
//
// Explicitly unsupported flags (rejected with exit code 2):
//
//	-r, --recursive              Recursive directory search — unbounded resource use; not permitted.
//	-R, --dereference-recursive  Like -r but follows symlinks — same reason.
//	-P, --perl-regexp            PCRE regex engine — ReDoS risk from backtracking; not permitted.
//	-f, --file=FILE              Read patterns from FILE — not implemented.
//	--color, --colour            Color output — no terminal in the safe shell.
//	-Z, --null                   Null-terminated output — not implemented.
//	-z, --null-data              Null-terminated input — not implemented.
//	-b, --byte-offset            Byte offsets — not implemented.
//	-d, --directories=ACTION     Directory action — not implemented.
//	-D, --devices=ACTION         Device action — not implemented.
//
// All other unknown flags are rejected automatically by pflag.
//
// Resource limits (prevents DoS against infinite/huge sources):
//
//	grepMaxLineBytes    = 256 KiB   maximum bytes stored per input line (excess silently dropped)
//	grepMaxContextLines = 10_000    maximum value of -A/-B/-C (larger values are clamped)

package builtins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

// Resource limits — prevent DoS via huge context requests or infinite sources.
const (
	grepMaxLineBytes    = 256 * 1024 // max bytes stored per input line (excess silently dropped)
	grepMaxContextLines = 10_000     // max -A/-B/-C value (larger values are clamped)
	grepUnlimited       = -1         // sentinel for maxCount: no match limit
)

// errGrepStopFile is a sentinel that stops processing the current file early (e.g. -l or -m).
var errGrepStopFile = errors.New("grep: stop processing this file")

// grepMatcher holds the compiled pattern and match configuration.
type grepMatcher struct {
	re          *regexp.Regexp
	invertMatch bool
}

// matches returns true if text matches the pattern (after applying invertMatch).
func (m *grepMatcher) matches(text string) bool {
	return m.re.MatchString(text) != m.invertMatch
}

// findAll returns all non-overlapping matches in text. Used for -o mode.
func (m *grepMatcher) findAll(text string) []string {
	return m.re.FindAllString(text, -1)
}

// grepBufLine stores one buffered before-context line.
type grepBufLine struct {
	lineNum int
	text    string
}

// grepLineState tracks context buffering and separator state for one file.
type grepLineState struct {
	beforeCap       int
	beforeBuf       []grepBufLine
	beforeHead      int
	beforeSize      int
	afterLeft       int
	lastOutLine     int  // 1-based line number of the last line written to output; 0 = none
	haveOutput      bool // whether any output has been written
	maxCountReached bool // true after -m N matches have been found; drain after-context then stop
}

func newGrepLineState(beforeCap int) *grepLineState {
	s := &grepLineState{beforeCap: beforeCap}
	if beforeCap > 0 {
		s.beforeBuf = make([]grepBufLine, beforeCap)
	}
	return s
}

// pushBefore adds a line to the before-context ring buffer.
func (s *grepLineState) pushBefore(lineNum int, text string) {
	if s.beforeCap == 0 {
		return
	}
	s.beforeBuf[s.beforeHead] = grepBufLine{lineNum: lineNum, text: text}
	s.beforeHead = (s.beforeHead + 1) % s.beforeCap
	if s.beforeSize < s.beforeCap {
		s.beforeSize++
	}
}

// drainBefore returns all lines in the before-context buffer (oldest first) and clears it.
func (s *grepLineState) drainBefore() []grepBufLine {
	if s.beforeSize == 0 {
		return nil
	}
	result := make([]grepBufLine, s.beforeSize)
	start := (s.beforeHead - s.beforeSize + s.beforeCap) % s.beforeCap
	for i := 0; i < s.beforeSize; i++ {
		result[i] = s.beforeBuf[(start+i)%s.beforeCap]
	}
	s.beforeSize = 0
	s.beforeHead = 0
	return result
}

// grepFileOpts carries per-file output configuration.
type grepFileOpts struct {
	matcher           *grepMatcher
	name              string // display name (may be "(standard input)")
	printFilename     bool
	lineNumber        bool
	count             bool
	filesWithMatches  bool
	filesWithoutMatch bool
	quiet             bool
	onlyMatching      bool
	maxCount          int  // grepUnlimited = no limit; 0 = stop after 0 matches; N = stop after N
	afterContext      int
	beforeContext     int
	contextMode       bool // true when any of -A/-B/-C was explicitly provided (enables '--' separator)
}

// builtinGrep implements the grep command.
func builtinGrep(ctx context.Context, callCtx *CallContext, args []string) Result {
	fs := pflag.NewFlagSet("grep", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var patterns []string
	fs.StringArrayVarP(&patterns, "regexp", "e", nil, "use PAT as a regular expression pattern")
	ignoreCase := fs.BoolP("ignore-case", "i", false, "ignore case distinctions")
	invertMatch := fs.BoolP("invert-match", "v", false, "select non-matching lines")
	lineNumber := fs.BoolP("line-number", "n", false, "print line number with output lines")
	count := fs.BoolP("count", "c", false, "print only a count of matching lines per file")
	filesWithMatches := fs.BoolP("files-with-matches", "l", false, "print only names of FILEs containing matches")
	filesWithoutMatch := fs.BoolP("files-without-match", "L", false, "print only names of FILEs containing no match")
	quietVal := false
	fs.BoolVarP(&quietVal, "quiet", "q", false, "suppress all normal output")
	fs.BoolVar(&quietVal, "silent", false, "suppress all normal output (alias for --quiet)")
	noMessages := fs.BoolP("no-messages", "s", false, "suppress error messages about nonexistent/unreadable files")
	wordRegexp := fs.BoolP("word-regexp", "w", false, "force PATTERN to match only whole words")
	lineRegexp := fs.BoolP("line-regexp", "x", false, "force PATTERN to match only whole lines")
	fixedStrings := fs.BoolP("fixed-strings", "F", false, "interpret PATTERN as a fixed string")
	_ = fs.BoolP("extended-regexp", "E", false, "use extended regular expression syntax (default; accepted for compatibility)")
	withFilename := fs.BoolP("with-filename", "H", false, "always print filename prefix")
	noFilenameFlag := fs.Bool("no-filename", false, "never print filename prefix")
	maxCountStr := fs.StringP("max-count", "m", "", "stop after N matches per file")
	onlyMatching := fs.BoolP("only-matching", "o", false, "print only the matched portion of a line")
	afterStr := fs.StringP("after-context", "A", "", "print N lines of trailing context")
	beforeStr := fs.StringP("before-context", "B", "", "print N lines of leading context")
	contextStr := fs.StringP("context", "C", "", "print N lines of output context (sets -A and -B)")
	help := fs.BoolP("help", "h", false, "print usage and exit")

	if err := fs.Parse(args); err != nil {
		callCtx.Errf("grep: %v\n", err)
		return Result{Code: 2}
	}

	if *help {
		callCtx.Out("Usage: grep [OPTION]... PATTERN [FILE]...\n\n")
		callCtx.Out("Search for PATTERN in each FILE or standard input.\n")
		callCtx.Out("With no FILE, or when FILE is -, read standard input.\n\n")
		fs.SetOutput(callCtx.Stdout)
		fs.PrintDefaults()
		return Result{}
	}

	// Collect positional args: first is the pattern (unless -e was used), rest are files.
	posArgs := fs.Args()
	if len(patterns) == 0 {
		if len(posArgs) == 0 {
			callCtx.Errf("grep: missing pattern\n")
			return Result{Code: 2}
		}
		patterns = []string{posArgs[0]}
		posArgs = posArgs[1:]
	}

	// Parse numeric arguments.
	afterCtx := 0
	beforeCtx := 0
	contextMode := false // enables '--' separator between non-adjacent output groups

	if fs.Changed("context") {
		n, err := parseGrepCount(*contextStr, "C")
		if err != nil {
			callCtx.Errf("grep: %v\n", err)
			return Result{Code: 2}
		}
		afterCtx = n
		beforeCtx = n
		contextMode = true
	}
	// -A and -B override -C when explicitly provided.
	if fs.Changed("after-context") {
		n, err := parseGrepCount(*afterStr, "A")
		if err != nil {
			callCtx.Errf("grep: %v\n", err)
			return Result{Code: 2}
		}
		afterCtx = n
		contextMode = true
	}
	if fs.Changed("before-context") {
		n, err := parseGrepCount(*beforeStr, "B")
		if err != nil {
			callCtx.Errf("grep: %v\n", err)
			return Result{Code: 2}
		}
		beforeCtx = n
		contextMode = true
	}

	maxCount := grepUnlimited // grepUnlimited = no match limit; 0 = stop after 0 matches
	if fs.Changed("max-count") {
		n, err := parseGrepCount(*maxCountStr, "m")
		if err != nil {
			callCtx.Errf("grep: %v\n", err)
			return Result{Code: 2}
		}
		maxCount = n
	}

	// Clamp context values.
	if afterCtx > grepMaxContextLines {
		afterCtx = grepMaxContextLines
	}
	if beforeCtx > grepMaxContextLines {
		beforeCtx = grepMaxContextLines
	}

	// Build the matcher.
	matcher, err := buildGrepMatcher(patterns, *fixedStrings, *ignoreCase, *wordRegexp, *lineRegexp, *invertMatch)
	if err != nil {
		callCtx.Errf("grep: %v\n", err)
		return Result{Code: 2}
	}

	files := posArgs
	if len(files) == 0 {
		files = []string{"-"}
	}

	// Determine default filename printing: on by default when >1 file.
	printFilename := len(files) > 1
	if *withFilename {
		printFilename = true
	}
	if *noFilenameFlag {
		printFilename = false
	}

	anyMatch := false     // true if any matching line was found (normal / -l / -c mode)
	anyLPrinted := false  // true if any file was printed in -L mode
	anyError := false

	for _, name := range files {
		matched, fileErr := func() (matched bool, fileErr error) {
			var rc io.ReadCloser
			displayName := name
			if name == "-" {
				if callCtx.Stdin == nil {
					return false, nil
				}
				rc = io.NopCloser(callCtx.Stdin)
				displayName = "(standard input)"
			} else {
				f, openErr := callCtx.OpenFile(ctx, name, os.O_RDONLY, 0)
				if openErr != nil {
					anyError = true
					if !*noMessages {
						callCtx.Errf("grep: %s: %v\n", name, openErr)
					}
					return false, nil
				}
				rc = f
				defer rc.Close()
			}

			opts := &grepFileOpts{
				matcher:           matcher,
				name:              displayName,
				printFilename:     printFilename,
				lineNumber:        *lineNumber,
				count:             *count,
				filesWithMatches:  *filesWithMatches,
				filesWithoutMatch: *filesWithoutMatch,
				quiet:             quietVal,
				onlyMatching:      *onlyMatching,
				maxCount:          maxCount,
				afterContext:      afterCtx,
				beforeContext:     beforeCtx,
				contextMode:       contextMode,
			}

			return grepFile(ctx, rc, callCtx.Stdout, opts)
		}()

		if fileErr != nil && fileErr != context.Canceled && fileErr != context.DeadlineExceeded {
			anyError = true
			if !*noMessages {
				callCtx.Errf("grep: %s: %v\n", name, fileErr)
			}
		}

		if *filesWithoutMatch {
			// For -L: a printed file is one with !matched; track that separately.
			if !matched {
				anyLPrinted = true
			}
		} else if matched {
			anyMatch = true
			if quietVal {
				return Result{}
			}
		}
	}

	if anyError {
		return Result{Code: 2}
	}
	if *filesWithoutMatch {
		// -L mode: exit 0 if at least one file was printed (had no matches), 1 otherwise.
		if anyLPrinted {
			return Result{}
		}
		return Result{Code: 1}
	}
	if anyMatch {
		return Result{}
	}
	return Result{Code: 1}
}

// grepFile processes a single reader, writing matches to w.
// Returns (matched=true, nil) if at least one match was found.
func grepFile(ctx context.Context, r io.Reader, w io.Writer, opts *grepFileOpts) (matched bool, err error) {
	state := newGrepLineState(opts.beforeContext)
	var lineNum int
	var matchCount int

	lineErr := grepReadLines(ctx, r, func(text string) error {
		lineNum++
		isMatch := opts.matcher.matches(text)

		// --count mode: just tally, no output.
		if opts.count {
			if isMatch {
				matchCount++
			}
			return nil
		}

		// --quiet mode: just detect first match.
		if opts.quiet {
			if isMatch {
				matched = true
				return errGrepStopFile
			}
			return nil
		}

		// --files-without-match mode: detect any match, suppress line output.
		if opts.filesWithoutMatch {
			if isMatch {
				matched = true
				return errGrepStopFile
			}
			return nil
		}

		// Handle context tracking and output for this line.
		return grepOutputLine(ctx, w, opts, state, lineNum, text, isMatch, &matched, &matchCount)
	})

	if lineErr != nil && lineErr != errGrepStopFile {
		err = lineErr
	}

	// Emit count output after all lines are processed.
	if opts.count {
		if matchCount > 0 {
			matched = true
		}
		if opts.quiet {
			return
		}
		if opts.printFilename {
			_, _ = fmt.Fprintf(w, "%s:%d\n", opts.name, matchCount)
		} else {
			_, _ = fmt.Fprintf(w, "%d\n", matchCount)
		}
		return
	}

	// -L: print filename only if no match was found.
	if opts.filesWithoutMatch && !matched && !opts.quiet {
		_, _ = fmt.Fprintf(w, "%s\n", opts.name)
	}

	return
}

// grepOutputLine handles output for a single line in normal (non-count, non-quiet) mode.
// It updates matched and matchCount via pointers.
func grepOutputLine(_ context.Context, w io.Writer, opts *grepFileOpts, state *grepLineState,
	lineNum int, text string, isMatch bool, matched *bool, matchCount *int) error {

	// If max count was already reached, drain remaining after-context lines then stop.
	if state.maxCountReached {
		if state.afterLeft > 0 {
			if err := grepWriteSep(w, state, opts, lineNum); err != nil {
				return err
			}
			if err := grepWriteLine(w, opts, lineNum, text, true); err != nil {
				return err
			}
			state.lastOutLine = lineNum
			state.haveOutput = true
			state.afterLeft--
			if state.afterLeft == 0 {
				return errGrepStopFile
			}
		} else {
			return errGrepStopFile
		}
		return nil
	}

	if isMatch {
		// -m 0: allow zero matches → stop immediately without printing.
		if opts.maxCount == 0 {
			return errGrepStopFile
		}

		// -l: stop after first match, print only the filename (no line content).
		if opts.filesWithMatches {
			*matched = true
			if err := grepPrintFilename(w, opts); err != nil {
				return err
			}
			return errGrepStopFile
		}

		// Flush before-context lines that have not yet been printed.
		beforeLines := state.drainBefore()
		for _, bl := range beforeLines {
			if bl.lineNum > state.lastOutLine {
				if err := grepWriteSep(w, state, opts, bl.lineNum); err != nil {
					return err
				}
				if err := grepWriteLine(w, opts, bl.lineNum, bl.text, true); err != nil {
					return err
				}
				state.lastOutLine = bl.lineNum
				state.haveOutput = true
			}
		}

		// Write the matching line.
		if err := grepWriteSep(w, state, opts, lineNum); err != nil {
			return err
		}
		if err := grepWriteLine(w, opts, lineNum, text, false); err != nil {
			return err
		}
		state.lastOutLine = lineNum
		state.haveOutput = true
		state.afterLeft = opts.afterContext
		*matched = true
		*matchCount++

		// -m N (N>0): after N matches, drain after-context if any, then stop.
		if opts.maxCount > 0 && *matchCount >= opts.maxCount {
			if opts.afterContext == 0 {
				return errGrepStopFile
			}
			state.maxCountReached = true
		}
	} else if state.afterLeft > 0 {
		// After-context line.
		if err := grepWriteSep(w, state, opts, lineNum); err != nil {
			return err
		}
		if err := grepWriteLine(w, opts, lineNum, text, true); err != nil {
			return err
		}
		state.lastOutLine = lineNum
		state.haveOutput = true
		state.afterLeft--
		// Also keep in before-buffer in case the next match needs before-context.
		state.pushBefore(lineNum, text)
	} else {
		// Non-matching, not in after-context: buffer for potential before-context.
		state.pushBefore(lineNum, text)
	}

	return nil
}

// grepWriteSep writes the "--" separator if there is a gap between
// the last output line and the current line being printed.
// The separator is only emitted when context mode is active (-A/-B/-C were provided).
func grepWriteSep(w io.Writer, state *grepLineState, opts *grepFileOpts, lineNum int) error {
	if !opts.contextMode {
		return nil
	}
	if state.haveOutput && lineNum > state.lastOutLine+1 {
		_, err := fmt.Fprintln(w, "--")
		return err
	}
	return nil
}

// grepWriteLine writes a single output line with the appropriate prefix.
// isCtx=true uses dash separators (context), false uses colon separators (match).
// For -o mode on match lines, each match is printed on its own output line.
func grepWriteLine(w io.Writer, opts *grepFileOpts, lineNum int, text string, isCtx bool) error {
	sep := ":"
	if isCtx {
		sep = "-"
	}

	// Build the prefix.
	var prefix string
	switch {
	case opts.printFilename && opts.lineNumber:
		prefix = fmt.Sprintf("%s%s%d%s", opts.name, sep, lineNum, sep)
	case opts.printFilename:
		prefix = opts.name + sep
	case opts.lineNumber:
		prefix = fmt.Sprintf("%d%s", lineNum, sep)
	}

	// -o mode: print only matching portions (applies to match lines only, not context).
	if opts.onlyMatching && !isCtx && !opts.matcher.invertMatch {
		matches := opts.matcher.findAll(text)
		for _, m := range matches {
			if _, err := fmt.Fprintf(w, "%s%s\n", prefix, m); err != nil {
				return err
			}
		}
		return nil
	}

	_, err := fmt.Fprintf(w, "%s%s\n", prefix, text)
	return err
}

// grepPrintFilename writes the filename (for -l mode).
func grepPrintFilename(w io.Writer, opts *grepFileOpts) error {
	_, err := fmt.Fprintf(w, "%s\n", opts.name)
	return err
}

// grepReadLines reads r line by line, calling fn with each stripped line (no terminator).
// Lines longer than grepMaxLineBytes are silently capped.
// The loop checks ctx.Err() before each read.
func grepReadLines(ctx context.Context, r io.Reader, fn func(text string) error) error {
	var buf [4096]byte
	var line []byte
	prevCR := false

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		nr, readErr := r.Read(buf[:])
		for i := 0; i < nr; i++ {
			b := buf[i]

			// Handle a CR that was at the end of the previous chunk.
			if prevCR {
				prevCR = false
				if b == '\n' {
					// CRLF split across chunk boundary: the CR already committed the line
					// and reset `line`. Skip this LF and start fresh.
					line = line[:0]
					continue
				}
				// Lone CR (already committed the line); `line` was reset, fall through
				// to process b as the first byte of the next line.
			}

			if b == '\r' {
				if i+1 < nr {
					if buf[i+1] == '\n' {
						// CRLF: consume now.
						i++
						if err := fn(string(line)); err != nil {
							return err
						}
						line = line[:0]
					} else {
						// Lone CR: emit line, reset.
						if err := fn(string(line)); err != nil {
							return err
						}
						line = line[:0]
					}
				} else {
					// CR at end of chunk: emit line, reset, mark prevCR so next chunk
					// can decide if this is CRLF.
					if err := fn(string(line)); err != nil {
						return err
					}
					line = line[:0]
					prevCR = true
				}
				continue
			}

			if b == '\n' {
				if err := fn(string(line)); err != nil {
					return err
				}
				line = line[:0]
				continue
			}

			line = appendGrepByte(line, b)
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	// Handle a trailing CR that was never followed by LF.
	// The line was already committed in the prevCR branch; nothing left in `line`.

	// Commit any trailing content without a newline.
	if len(line) > 0 {
		return fn(string(line))
	}

	return nil
}

// appendGrepByte appends b to line, capping at grepMaxLineBytes.
func appendGrepByte(line []byte, b byte) []byte {
	if len(line) < grepMaxLineBytes {
		return append(line, b)
	}
	return line
}

// parseGrepCount parses a non-negative integer string for -m/-A/-B/-C.
// On overflow it clamps to math.MaxInt.
func parseGrepCount(s, flag string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("invalid argument for -%s: %q", flag, s)
	}
	if s[0] == '-' {
		return 0, fmt.Errorf("invalid argument for -%s: %q (negative values not allowed)", flag, s)
	}
	n64, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
			return math.MaxInt, nil
		}
		return 0, fmt.Errorf("invalid argument for -%s: %q", flag, s)
	}
	if n64 > math.MaxInt {
		return math.MaxInt, nil
	}
	return int(n64), nil
}

// buildGrepMatcher compiles the set of patterns into a single matcher.
// Applies -F (fixed), -i (case-insensitive), -w (word), -x (line) modifiers.
func buildGrepMatcher(patterns []string, fixedStrings, ignoreCase, wordRegexp, lineRegexp, invertMatch bool) (*grepMatcher, error) {
	if len(patterns) == 0 {
		return nil, fmt.Errorf("no pattern provided")
	}

	parts := make([]string, len(patterns))
	for i, p := range patterns {
		if fixedStrings {
			p = regexp.QuoteMeta(p)
		}
		if wordRegexp {
			p = `\b(?:` + p + `)\b`
		}
		if lineRegexp {
			p = `^(?:` + p + `)$`
		} else {
			p = `(?:` + p + `)`
		}
		parts[i] = p
	}

	combined := strings.Join(parts, "|")
	if ignoreCase {
		combined = `(?i)` + combined
	}

	re, err := regexp.Compile(combined)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %v", err)
	}

	return &grepMatcher{re: re, invertMatch: invertMatch}, nil
}
