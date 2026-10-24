// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package cut implements the cut builtin command.
//
// cut — remove sections from each line of files
//
// Usage: cut OPTION... [FILE]...
//
// Print selected parts of lines from each FILE to standard output.
// With no FILE, or when FILE is -, read standard input.
//
// Exactly one of -b, -c, or -f must be specified.
//
// Accepted flags:
//
//	-b LIST, --bytes=LIST
//	    Select only these bytes. LIST is a comma-separated set of byte
//	    positions and ranges (e.g. 1,3-5,7-). Positions are 1-based.
//
//	-c LIST, --characters=LIST
//	    Select only these characters (treated as bytes, matching GNU cut).
//	    Same list format as -b.
//
//	-d DELIM, --delimiter=DELIM
//	    Use DELIM instead of TAB for field delimiter. Used with -f.
//
//	-f LIST, --fields=LIST
//	    Select only these fields, separated by the delimiter character.
//	    Same list format as -b.
//
//	-n
//	    (ignored) Accepted for POSIX compatibility but has no effect,
//	    matching GNU coreutils behavior.
//
//	-s, --only-delimited
//	    Do not print lines not containing delimiters (only with -f).
//
//	--complement
//	    Complement the set of selected bytes, characters, or fields.
//
//	--output-delimiter=STRING
//	    Use STRING as the output delimiter. The default is the input
//	    delimiter.
//
//	--help
//	    Print this usage message to stdout and exit 0.
//
// Exit codes:
//
//	0  All files processed successfully.
//	1  At least one error occurred (missing file, invalid argument, etc.).
//
// Memory safety:
//
//	Lines are read via a streaming scanner with a per-line cap of
//	MaxLineBytes (1 MiB). Lines exceeding this cap produce an error
//	rather than an unbounded allocation. All loops check ctx.Err()
//	at each iteration to honour the shell's execution timeout.
package cut

import (
	"bufio"
	"context"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the cut builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "cut",
	Description: "remove sections from each line",
	MakeFlags:   registerFlags,
}

// MaxLineBytes is the per-line buffer cap for the line scanner.
const MaxLineBytes = 1 << 20 // 1 MiB

// mode distinguishes the three mutually exclusive selection modes.
type mode int

const (
	modeNone   mode = iota
	modeBytes       // -b
	modeChars       // -c
	modeFields      // -f
)

// registerFlags registers all cut flags on the framework-provided FlagSet and
// returns a bound handler whose flag variables are captured by closure.
func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.Bool("help", false, "print usage and exit")
	bytesListStr := fs.StringP("bytes", "b", "", "select only these bytes")
	charsListStr := fs.StringP("characters", "c", "", "select only these characters")
	fieldsListStr := fs.StringP("fields", "f", "", "select only these fields")
	delimiter := fs.StringP("delimiter", "d", "\t", "use DELIM instead of TAB for field delimiter")
	onlyDelimited := fs.BoolP("only-delimited", "s", false, "do not print lines not containing delimiters")
	_ = fs.BoolP("", "n", false, "do not split multi-byte characters")
	complement := fs.Bool("complement", false, "complement the set of selected bytes, characters, or fields")
	outputDelimiter := fs.String("output-delimiter", "", "use STRING as the output delimiter")

	return func(ctx context.Context, callCtx *builtins.CallContext, files []string) builtins.Result {
		// Validate -d's value BEFORE the --help short-circuit so that
		// `cut -d xy --help` exits with "delimiter must be a single
		// character" instead of printing help. GNU cut runs this check
		// during option processing; the shared args-trim in
		// builtins/builtins.go relies on this ordering to safely honour
		// `--help` after value-taking flags. Only validate when -d was
		// actually set so `cut --help` (no -d, default "\t") still
		// short-circuits cleanly. GNU appends `Try 'cut --help' for
		// more information.` after this specific error.
		if fs.Changed("delimiter") && len(*delimiter) != 1 {
			callCtx.Errf("cut: the delimiter must be a single character\n")
			callCtx.Errf("Try 'cut --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		if *help {
			callCtx.Out("Usage: cut OPTION... [FILE]...\n")
			callCtx.Out("Print selected parts of lines from each FILE to standard output.\n")
			callCtx.Out("With no FILE, or when FILE is -, read standard input.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		// Determine mode: exactly one of -b, -c, -f must be specified.
		// Use fs.Changed() to detect whether the flag was explicitly provided,
		// rather than comparing the value to "" (which would miss -b "").
		var m mode
		var listStr string
		modeCount := 0
		if fs.Changed("bytes") {
			m = modeBytes
			listStr = *bytesListStr
			modeCount++
		}
		if fs.Changed("characters") {
			m = modeChars
			listStr = *charsListStr
			modeCount++
		}
		if fs.Changed("fields") {
			m = modeFields
			listStr = *fieldsListStr
			modeCount++
		}
		if modeCount == 0 {
			callCtx.Errf("cut: you must specify a list of bytes, characters, or fields\n")
			return builtins.Result{Code: 1}
		}
		if modeCount > 1 {
			callCtx.Errf("cut: only one type of list may be specified\n")
			return builtins.Result{Code: 1}
		}

		// -d and -s are only valid with -f.
		if m != modeFields {
			if fs.Changed("delimiter") {
				callCtx.Errf("cut: an input delimiter may be specified only when operating on fields\n")
				return builtins.Result{Code: 1}
			}
			if *onlyDelimited {
				callCtx.Errf("cut: suppressing non-delimited lines makes sense\n\tonly when operating on fields\n")
				return builtins.Result{Code: 1}
			}
		}

		// Delimiter length was already validated above (before the
		// --help short-circuit). When -d wasn't set, the default "\t"
		// is a single byte and trivially satisfies the constraint.
		delimByte := (*delimiter)[0]

		// Parse the list.
		ranges, err := parseList(listStr)
		if err != nil {
			callCtx.Errf("cut: %s\n", err.Error())
			return builtins.Result{Code: 1}
		}

		// Determine output delimiter.
		outDelim := *delimiter
		outDelimSet := fs.Changed("output-delimiter")
		if outDelimSet {
			outDelim = *outputDelimiter
		}

		cfg := &cutConfig{
			mode:          m,
			ranges:        ranges,
			delimByte:     delimByte,
			onlyDelimited: *onlyDelimited,
			complement:    *complement,
			outDelim:      outDelim,
			outDelimSet:   outDelimSet,
		}

		// Default to stdin when no file arguments were given.
		if len(files) == 0 {
			files = []string{"-"}
		}

		var failed bool
		for _, file := range files {
			if ctx.Err() != nil {
				break
			}
			if err := processFile(ctx, callCtx, file, cfg); err != nil {
				name := file
				if file == "-" {
					name = "standard input"
				}
				callCtx.Errf("cut: %s: %s\n", name, callCtx.PortableErr(err))
				failed = true
			}
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

// newline is a package-level buffer reused for every line-terminator Write,
// avoiding a heap allocation per line.
var newline = []byte{'\n'}

// cutConfig holds the parsed configuration for a cut invocation.
type cutConfig struct {
	mode          mode
	ranges        [][2]int // sorted, merged, 1-based inclusive ranges
	delimByte     byte
	onlyDelimited bool
	complement    bool
	outDelim      string
	outDelimSet   bool
}

// parseList parses a comma-separated list of ranges/positions into sorted,
// merged [2]int ranges (1-based inclusive). Open-ended ranges use
// math.MaxInt32 as sentinel.
func parseList(s string) ([][2]int, error) {
	parts := strings.Split(s, ",")
	var ranges [][2]int
	for _, part := range parts {
		if part == "" {
			return nil, invalidRange(s)
		}
		dashIdx := strings.IndexByte(part, '-')
		if dashIdx < 0 {
			// Single number: N
			n, err := strconv.Atoi(part)
			if err != nil || n <= 0 {
				return nil, invalidRange(part)
			}
			ranges = append(ranges, [2]int{n, n})
		} else {
			left := part[:dashIdx]
			right := part[dashIdx+1:]
			// A bare "-" (both sides empty) is invalid.
			if left == "" && right == "" {
				return nil, invalidRange(part)
			}
			var start, end int
			if left == "" {
				start = 1
			} else {
				var err error
				start, err = strconv.Atoi(left)
				if err != nil || start <= 0 {
					return nil, invalidRange(part)
				}
			}
			if right == "" {
				end = math.MaxInt32
			} else {
				var err error
				end, err = strconv.Atoi(right)
				if err != nil || end <= 0 {
					return nil, invalidRange(part)
				}
			}
			if start > end {
				return nil, invalidDecreasingRange(part)
			}
			ranges = append(ranges, [2]int{start, end})
		}
	}
	if len(ranges) == 0 {
		return nil, invalidRange(s)
	}

	// Sort by start, then merge overlapping/adjacent.
	slices.SortFunc(ranges, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})

	// Merge overlapping ranges (but not merely adjacent ones, so that
	// --output-delimiter can be inserted between adjacent ranges like 1-2,3-4).
	merged := [][2]int{ranges[0]}
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r[0] <= last[1] {
			// Truly overlapping: extend.
			if r[1] > last[1] {
				last[1] = r[1]
			}
		} else {
			merged = append(merged, r)
		}
	}
	return merged, nil
}

func invalidRange(s string) error {
	return cutError("invalid byte, character or field list: " + s)
}

func invalidDecreasingRange(s string) error {
	return cutError("invalid decreasing range: " + s)
}

// cutError is a simple error type.
type cutError string

func (e cutError) Error() string { return string(e) }

// processFile opens and processes one file (or stdin for "-").
func processFile(ctx context.Context, callCtx *builtins.CallContext, file string, cfg *cutConfig) error {
	var rc io.ReadCloser
	if file == "-" {
		if callCtx.Stdin == nil {
			return nil
		}
		rc = io.NopCloser(callCtx.Stdin)
	} else {
		f, err := callCtx.OpenFile(ctx, file, os.O_RDONLY, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		rc = f
	}

	sc := bufio.NewScanner(rc)
	buf := make([]byte, 4096)
	sc.Buffer(buf, MaxLineBytes)
	sc.Split(scanLinesPreservingNewline)

	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := sc.Bytes()
		// Strip trailing newline for processing; we always add one back.
		raw := stripNewline(line)
		switch cfg.mode {
		case modeBytes, modeChars:
			// GNU coreutils treats -c identically to -b (byte-wise selection).
			processBytes(callCtx, raw, cfg)
		case modeFields:
			processFields(callCtx, raw, cfg)
		}
	}
	return sc.Err()
}

// stripNewline removes a trailing \n from a byte slice.
// Only \n is stripped — \r is preserved as a regular content byte,
// matching GNU cut behavior where \r is not part of the line terminator.
func stripNewline(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b
}

// inRanges checks whether pos (1-based) falls within any of the sorted ranges.
func inRanges(pos int, ranges [][2]int) bool {
	for _, r := range ranges {
		if pos < r[0] {
			return false // ranges are sorted, no need to continue
		}
		if pos <= r[1] {
			return true
		}
	}
	return false
}

// processBytes selects bytes from a line.
func processBytes(callCtx *builtins.CallContext, raw []byte, cfg *cutConfig) {
	n := len(raw)
	if n == 0 {
		callCtx.Out("\n")
		return
	}

	if cfg.complement {
		// Select bytes NOT in ranges.
		if cfg.outDelimSet {
			processBytesComplementWithOutDelim(callCtx, raw, cfg)
		} else {
			start := -1
			for i := range n {
				if !inRanges(i+1, cfg.ranges) {
					if start < 0 {
						start = i
					}
				} else {
					if start >= 0 {
						callCtx.Stdout.Write(raw[start:i]) //nolint:errcheck
						start = -1
					}
				}
			}
			if start >= 0 {
				callCtx.Stdout.Write(raw[start:]) //nolint:errcheck
			}
		}
	} else {
		if cfg.outDelimSet {
			processBytesWithOutDelim(callCtx, raw, cfg)
		} else {
			start := -1
			for i := range n {
				if inRanges(i+1, cfg.ranges) {
					if start < 0 {
						start = i
					}
				} else {
					if start >= 0 {
						callCtx.Stdout.Write(raw[start:i]) //nolint:errcheck
						start = -1
					}
				}
			}
			if start >= 0 {
				callCtx.Stdout.Write(raw[start:]) //nolint:errcheck
			}
		}
	}
	callCtx.Stdout.Write(newline) //nolint:errcheck
}

// processBytesWithOutDelim outputs selected byte ranges with the output
// delimiter inserted between non-contiguous ranges.
func processBytesWithOutDelim(callCtx *builtins.CallContext, raw []byte, cfg *cutConfig) {
	n := len(raw)
	first := true
	for _, r := range cfg.ranges {
		start := r[0]
		end := r[1]
		if start > n {
			break
		}
		if end > n {
			end = n
		}
		if !first {
			callCtx.Out(cfg.outDelim)
		}
		_, _ = callCtx.Stdout.Write(raw[start-1 : end])
		first = false
	}
}

// processBytesComplementWithOutDelim outputs complemented byte ranges with output delimiter.
func processBytesComplementWithOutDelim(callCtx *builtins.CallContext, raw []byte, cfg *cutConfig) {
	compRanges := complementRanges(cfg.ranges, len(raw))
	first := true
	for _, r := range compRanges {
		if !first {
			callCtx.Out(cfg.outDelim)
		}
		_, _ = callCtx.Stdout.Write(raw[r[0]-1 : r[1]])
		first = false
	}
}

// processFields selects fields from a line.
func processFields(callCtx *builtins.CallContext, raw []byte, cfg *cutConfig) {
	hasDelim := false
	for _, b := range raw {
		if b == cfg.delimByte {
			hasDelim = true
			break
		}
	}
	if !hasDelim {
		if cfg.onlyDelimited {
			return
		}
		callCtx.Stdout.Write(raw)     //nolint:errcheck
		callCtx.Stdout.Write(newline) //nolint:errcheck
		return
	}

	nFields := 1
	for _, b := range raw {
		if b == cfg.delimByte {
			nFields++
		}
	}

	fieldIdx := 0
	fieldStart := 0
	firstOutput := true

	for i := 0; i <= len(raw); i++ {
		if i < len(raw) && raw[i] != cfg.delimByte {
			continue
		}
		fieldIdx++
		fieldNum := fieldIdx

		selected := false
		if cfg.complement {
			selected = !inRanges(fieldNum, cfg.ranges)
		} else {
			selected = inRanges(fieldNum, cfg.ranges)
		}

		if selected {
			if !firstOutput {
				callCtx.Out(cfg.outDelim)
			}
			callCtx.Stdout.Write(raw[fieldStart:i]) //nolint:errcheck
			firstOutput = false
		}

		fieldStart = i + 1
	}
	callCtx.Stdout.Write(newline) //nolint:errcheck
}

// complementRanges returns the complement of the given sorted, merged ranges
// within [1, total].
func complementRanges(ranges [][2]int, total int) [][2]int {
	var result [][2]int
	next := 1
	for _, r := range ranges {
		start := r[0]
		end := r[1]
		if start > total {
			break
		}
		if next < start {
			result = append(result, [2]int{next, start - 1})
		}
		if end >= total {
			next = total + 1
			break
		}
		next = end + 1
	}
	if next <= total {
		result = append(result, [2]int{next, total})
	}
	return result
}

// scanLinesPreservingNewline is a bufio.SplitFunc that includes the line
// terminator (\n) in the returned token. Unlike bufio.ScanLines, it does not
// strip \r\n or \n, so the caller reproduces the exact file content. If the
// file's last line has no terminator, the bare bytes are returned as the
// final token.
func scanLinesPreservingNewline(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' {
			return i + 1, data[:i+1], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
