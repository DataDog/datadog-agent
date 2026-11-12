// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package uniq implements the uniq builtin command.
//
// uniq — report or omit repeated lines
//
// Usage: uniq [OPTION]... [INPUT_FILE]
//
// Filter adjacent matching lines from INPUT_FILE (or standard input),
// writing to standard output.
//
// With no INPUT_FILE, or when INPUT_FILE is -, read standard input.
//
// Note: the output file argument (second positional arg) supported by
// GNU uniq is intentionally NOT implemented because it writes to the
// filesystem, violating the shell's safety rules.
//
// Accepted flags:
//
//	-c, --count
//	    Prefix lines by the number of occurrences.
//
//	-d, --repeated
//	    Only print duplicate lines, one for each group.
//
//	-D, --all-repeated[=METHOD]
//	    Print all duplicate lines. METHOD={none,prepend,separate}
//	    (default: none). Mutually exclusive with --group.
//
//	-u, --unique
//	    Only print unique lines (lines that are not repeated).
//
//	-i, --ignore-case
//	    Ignore differences in case when comparing lines.
//
//	-f N, --skip-fields=N
//	    Avoid comparing the first N fields. Fields are sequences of
//	    non-blank characters separated by blanks (spaces and tabs).
//
//	-s N, --skip-chars=N
//	    Avoid comparing the first N characters (applied after field
//	    skipping).
//
//	-w N, --check-chars=N
//	    Compare no more than N characters in each line.
//
//	-z, --zero-terminated
//	    Line delimiter is NUL (\0), not newline.
//
//	--group[=METHOD]
//	    Show all input lines, separating groups with an empty line.
//	    METHOD={separate,prepend,append,both} (default: separate).
//	    Mutually exclusive with -c, -d, -D, -u.
//
//	-h, --help
//	    Print this usage message to stdout and exit 0.
//
// Exit codes:
//
//	0  Success.
//	1  An error occurred (invalid argument, missing file, incompatible flags).
//
// Memory safety:
//
//	Lines are processed one at a time via a streaming scanner with a
//	per-line cap of MaxLineBytes (1 MiB). Only the current and previous
//	lines are kept in memory. All loops check ctx.Err() to honour the
//	shell's execution timeout.
package uniq

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the uniq builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "uniq",
	Description: "report or omit repeated lines",
	MakeFlags:   registerFlags,
}

// MaxLineBytes is the per-line buffer cap for the line scanner.
const MaxLineBytes = 1 << 20 // 1 MiB

// MaxCount is the maximum accepted value for -f, -s, -w flags.
const MaxCount = 1<<31 - 1 // 2 147 483 647

// countFieldWidth is the width of the count prefix produced by -c.
const countFieldWidth = 7

// initialBufSize is the starting buffer size for the scanner.
const initialBufSize = 4096

// groupMethod controls how --group inserts blank-line separators.
type groupMethod int

const (
	groupSeparate groupMethod = iota
	groupPrepend
	groupAppend
	groupBoth
)

// allRepeatedMethod controls how -D/--all-repeated delimits groups.
type allRepeatedMethod int

const (
	allRepeatedNone allRepeatedMethod = iota
	allRepeatedPrepend
	allRepeatedSeparate
)

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.BoolP("help", "h", false, "print usage and exit")
	count := fs.BoolP("count", "c", false, "prefix lines by the number of occurrences")
	repeated := fs.BoolP("repeated", "d", false, "only print duplicate lines, one for each group")
	unique := fs.BoolP("unique", "u", false, "only print unique lines")
	ignoreCase := fs.BoolP("ignore-case", "i", false, "ignore differences in case when comparing")
	zeroTerminated := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")

	skipFieldsStr := fs.StringP("skip-fields", "f", "0", "avoid comparing the first N fields")
	skipCharsStr := fs.StringP("skip-chars", "s", "0", "avoid comparing the first N characters")
	checkCharsStr := fs.StringP("check-chars", "w", "", "compare no more than N characters")

	allRepeatedStr := fs.StringP("all-repeated", "D", "", "print all duplicate lines; METHOD={none,prepend,separate}")
	groupStr := fs.String("group", "", "show all input lines with group separators; METHOD={separate,prepend,append,both}")

	fs.Lookup("all-repeated").NoOptDefVal = "none"
	fs.Lookup("group").NoOptDefVal = "separate"

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		// Validate value-taker inputs BEFORE the --help short-circuit
		// so that `uniq -f nope --help` exits with "invalid number of
		// fields to skip" instead of printing help. GNU uniq runs these
		// checks during option processing; the shared args-trim in
		// builtins/builtins.go relies on this ordering to safely honour
		// `--help` after value-taking flags. Default values ("0") pass
		// the checks so `uniq --help` (no flags) still short-circuits.
		skipFields, ok := parseNonNegativeInt(*skipFieldsStr)
		if !ok {
			callCtx.Errf("uniq: %s: invalid number of fields to skip\n", *skipFieldsStr)
			return builtins.Result{Code: 1}
		}
		skipChars, ok := parseNonNegativeInt(*skipCharsStr)
		if !ok {
			callCtx.Errf("uniq: %s: invalid number of bytes to skip\n", *skipCharsStr)
			return builtins.Result{Code: 1}
		}
		checkChars := int64(-1)
		if fs.Changed("check-chars") {
			checkChars, ok = parseNonNegativeInt(*checkCharsStr)
			if !ok {
				callCtx.Errf("uniq: %s: invalid number of bytes to compare\n", *checkCharsStr)
				return builtins.Result{Code: 1}
			}
		}

		if *help {
			callCtx.Out("Usage: uniq [OPTION]... [INPUT]\n")
			callCtx.Out("Filter adjacent matching lines from INPUT (or stdin),\n")
			callCtx.Out("writing to standard output.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		useAllRepeated := fs.Changed("all-repeated")
		arMethod := allRepeatedNone
		if useAllRepeated {
			var err error
			arMethod, err = parseAllRepeatedMethod(*allRepeatedStr)
			if err != nil {
				callCtx.Errf("uniq: %v\n", err)
				return builtins.Result{Code: 1}
			}
		}

		useGroup := fs.Changed("group")
		grpMethod := groupSeparate
		if useGroup {
			var err error
			grpMethod, err = parseGroupMethod(*groupStr)
			if err != nil {
				callCtx.Errf("uniq: %v\n", err)
				return builtins.Result{Code: 1}
			}
		}

		if useGroup && (*count || *repeated || useAllRepeated || *unique) {
			callCtx.Errf("uniq: --group is mutually exclusive with -c/-d/-D/-u\n")
			callCtx.Errf("Try 'uniq --help' for more information.\n")
			return builtins.Result{Code: 1}
		}
		if useAllRepeated && *count {
			callCtx.Errf("uniq: printing all duplicated lines and repeat counts is meaningless\n")
			callCtx.Errf("Try 'uniq --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		if len(args) > 1 {
			callCtx.Errf("uniq: extra operand '%s'\n", builtins.SafeOperand(args[1]))
			callCtx.Errf("Try 'uniq --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		file := "-"
		if len(args) == 1 {
			file = args[0]
		}

		var rc io.ReadCloser
		if file == "-" {
			if callCtx.Stdin == nil {
				return builtins.Result{}
			}
			rc = io.NopCloser(callCtx.Stdin)
		} else {
			f, err := callCtx.OpenFile(ctx, file, os.O_RDONLY, 0)
			if err != nil {
				callCtx.Errf("uniq: %s: %s\n", file, callCtx.PortableErr(err))
				return builtins.Result{Code: 1}
			}
			defer f.Close()
			rc = f
		}

		delim := byte('\n')
		if *zeroTerminated {
			delim = 0
		}

		// GNU uniq: --all-repeated --unique collapses to -d behavior (one per
		// duplicate group). Downgrade to the standard repeated path.
		if useAllRepeated && *unique {
			useAllRepeated = false
			*repeated = true
			*unique = false
		}

		cfg := &uniqConfig{
			count:          *count,
			repeated:       *repeated,
			unique:         *unique,
			ignoreCase:     *ignoreCase,
			skipFields:     skipFields,
			skipChars:      skipChars,
			checkChars:     checkChars,
			delim:          delim,
			useAllRepeated: useAllRepeated,
			arMethod:       arMethod,
			useGroup:       useGroup,
			grpMethod:      grpMethod,
		}

		if err := processInput(ctx, callCtx, rc, cfg); err != nil {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

type uniqConfig struct {
	count          bool
	repeated       bool
	unique         bool
	ignoreCase     bool
	skipFields     int64
	skipChars      int64
	checkChars     int64
	delim          byte
	useAllRepeated bool
	arMethod       allRepeatedMethod
	useGroup       bool
	grpMethod      groupMethod
}

func processInput(ctx context.Context, callCtx *builtins.CallContext, r io.Reader, cfg *uniqConfig) error {
	sc := bufio.NewScanner(r)
	buf := make([]byte, initialBufSize)
	sc.Buffer(buf, MaxLineBytes)
	sc.Split(makeSplitFunc(cfg.delim))

	w := callCtx.Stdout

	reportWrite := func(err error) error {
		if err != nil {
			callCtx.Errf("uniq: write error\n")
		}
		return err
	}

	writeLine := func(line []byte) error {
		if _, err := w.Write(line); err != nil {
			return err
		}
		_, err := w.Write([]byte{cfg.delim})
		return err
	}

	var prevLine []byte
	var prevKey []byte
	var lineCount int64
	first := true
	groupNum := 0

	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		curBytes := sc.Bytes()
		curKey := compareKeyBytes(curBytes, cfg)

		if first {
			prevLine = append(prevLine[:0], curBytes...)
			prevKey = append(prevKey[:0], curKey...)
			lineCount = 1
			first = false

			if cfg.useGroup {
				if cfg.grpMethod == groupPrepend || cfg.grpMethod == groupBoth {
					if err := reportWrite(writeLine(nil)); err != nil {
						return err
					}
				}
				if err := reportWrite(writeLine(prevLine)); err != nil {
					return err
				}
			}
			continue
		}

		same := bytes.Equal(prevKey, curKey)

		if same {
			if lineCount < math.MaxInt64 {
				lineCount++
			}
			if cfg.useGroup {
				if err := reportWrite(writeLine(curBytes)); err != nil {
					return err
				}
			} else if cfg.useAllRepeated {
				if lineCount == 2 {
					if groupNum > 0 && cfg.arMethod != allRepeatedNone {
						if err := reportWrite(writeLine(nil)); err != nil {
							return err
						}
					}
					if groupNum == 0 && cfg.arMethod == allRepeatedPrepend {
						if err := reportWrite(writeLine(nil)); err != nil {
							return err
						}
					}
					if err := reportWrite(writeLine(prevLine)); err != nil {
						return err
					}
					groupNum++
				}
				if err := reportWrite(writeLine(curBytes)); err != nil {
					return err
				}
			}
		} else {
			if cfg.useGroup {
				if err := reportWrite(writeLine(nil)); err != nil {
					return err
				}
				if err := reportWrite(writeLine(curBytes)); err != nil {
					return err
				}
				groupNum++
			} else if !cfg.useAllRepeated {
				if err := reportWrite(emitStandard(w, cfg, prevLine, lineCount)); err != nil {
					return err
				}
			}
			prevLine = append(prevLine[:0], curBytes...)
			prevKey = append(prevKey[:0], curKey...)
			lineCount = 1
		}
	}

	if err := sc.Err(); err != nil {
		callCtx.Errf("uniq: %s\n", callCtx.PortableErr(err))
		return err
	}

	if first {
		return nil
	}

	// Flush last group.
	if cfg.useGroup {
		if cfg.grpMethod == groupAppend || cfg.grpMethod == groupBoth {
			return reportWrite(writeLine(nil))
		}
		return nil
	}
	if cfg.useAllRepeated {
		return nil
	}
	return reportWrite(emitStandard(w, cfg, prevLine, lineCount))
}

func emitStandard(w io.Writer, cfg *uniqConfig, line []byte, count int64) error {
	if cfg.repeated && cfg.unique {
		return nil
	}
	if cfg.repeated && count < 2 {
		return nil
	}
	if cfg.unique && count >= 2 {
		return nil
	}
	if cfg.count {
		s := strconv.FormatInt(count, 10)
		for len(s) < countFieldWidth {
			s = " " + s
		}
		if _, err := io.WriteString(w, s+" "); err != nil {
			return err
		}
		if _, err := w.Write(line); err != nil {
			return err
		}
		_, err := w.Write([]byte{cfg.delim})
		return err
	}
	if _, err := w.Write(line); err != nil {
		return err
	}
	_, err := w.Write([]byte{cfg.delim})
	return err
}

// compareKeyBytes extracts the portion of line used for comparison, applying
// field skipping, char skipping, check-chars, and case folding.
// For the ignore-case path it returns a newly allocated lowercased copy;
// otherwise it returns a subslice of line (no allocation).
func compareKeyBytes(line []byte, cfg *uniqConfig) []byte {
	s := line
	if cfg.skipFields > 0 {
		s = skipFieldsBytesN(s, cfg.skipFields)
	}
	if cfg.skipChars > 0 && len(s) > 0 {
		skip := cfg.skipChars
		if skip > int64(len(s)) {
			skip = int64(len(s))
		}
		s = s[skip:]
	}
	if cfg.checkChars >= 0 && cfg.checkChars < int64(len(s)) {
		s = s[:cfg.checkChars]
	}
	if cfg.ignoreCase {
		s = asciiToLowerBytes(s)
	}
	return s
}

// asciiToLowerBytes folds only ASCII A-Z to a-z in a byte slice, matching GNU
// uniq behavior in the default C/POSIX locale. It always returns a new copy.
func asciiToLowerBytes(s []byte) []byte {
	b := make([]byte, len(s))
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return b
}

// skipFieldsBytesN skips the first n blank-delimited fields in a byte slice
// and returns the remainder, starting immediately after the last character
// of the n-th field (before any subsequent blanks).
func skipFieldsBytesN(s []byte, n int64) []byte {
	i := 0
	for field := int64(0); field < n && i < len(s); field++ {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}
	}
	return s[i:]
}

func parseNonNegativeInt(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			// Reject negative overflow (e.g. -999999999999999999999).
			if len(s) > 0 && s[0] == '-' {
				return 0, false
			}
			return MaxCount, true
		}
		return 0, false
	}
	if n < 0 {
		return 0, false
	}
	if n > MaxCount {
		n = MaxCount
	}
	return n, true
}

// parseAllRepeatedMethod parses the METHOD for --all-repeated.
// Cases are ordered deliberately: first match wins for prefix abbreviation,
// matching GNU coreutils behavior. If adding new options that share a prefix
// with existing ones, add explicit ambiguity detection.
func parseAllRepeatedMethod(s string) (allRepeatedMethod, error) {
	switch {
	case s == "":
		return 0, &invalidArgError{arg: s, flag: "--all-repeated", valid: []string{"none", "prepend", "separate"}}
	case strings.HasPrefix("none", s):
		return allRepeatedNone, nil
	case strings.HasPrefix("prepend", s):
		return allRepeatedPrepend, nil
	case strings.HasPrefix("separate", s):
		return allRepeatedSeparate, nil
	}
	return 0, &invalidArgError{arg: s, flag: "--all-repeated", valid: []string{"none", "prepend", "separate"}}
}

// parseGroupMethod parses the METHOD for --group.
// Cases are ordered deliberately: first match wins for prefix abbreviation,
// matching GNU coreutils behavior. If adding new options that share a prefix
// with existing ones, add explicit ambiguity detection.
func parseGroupMethod(s string) (groupMethod, error) {
	switch {
	case s == "":
		return 0, &invalidArgError{arg: s, flag: "--group", valid: []string{"prepend", "append", "separate", "both"}}
	case strings.HasPrefix("separate", s):
		return groupSeparate, nil
	case strings.HasPrefix("prepend", s):
		return groupPrepend, nil
	case strings.HasPrefix("append", s):
		return groupAppend, nil
	case strings.HasPrefix("both", s):
		return groupBoth, nil
	}
	return 0, &invalidArgError{arg: s, flag: "--group", valid: []string{"prepend", "append", "separate", "both"}}
}

type invalidArgError struct {
	arg   string
	flag  string
	valid []string
}

func (e *invalidArgError) Error() string {
	msg := "invalid argument '" + e.arg + "' for '" + e.flag + "'\nValid arguments are:\n"
	var arguments strings.Builder
	for _, v := range e.valid {
		arguments.WriteString("  - '" + v + "'\n")
	}
	msg += arguments.String()
	return msg
}

// makeSplitFunc returns a bufio.SplitFunc that splits on the given delimiter.
// The token returned does NOT include the trailing delimiter.
func makeSplitFunc(delim byte) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		for i, b := range data {
			if b == delim {
				return i + 1, data[:i], nil
			}
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}
}
