// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package wc implements the wc builtin command.
//
// wc — print newline, word, and byte counts for each file
//
// Usage: wc [OPTION]... [FILE]...
//
// Print newline, word, and byte counts for each FILE, and a total line
// if more than one FILE is specified. A word is a non-zero-length sequence
// of characters delimited by white space. With no FILE, or when FILE is -,
// read standard input.
//
// When no flags are given, -l, -w, and -c are assumed (lines, words, bytes).
//
// Accepted flags:
//
//	-l, --lines
//	    Print the newline count.
//
//	-w, --words
//	    Print the word count.
//
//	-c, --bytes
//	    Print the byte count.
//
//	-m, --chars
//	    Print the character count. In a multibyte locale, the number of
//	    characters may differ from the number of bytes.
//
//	-L, --max-line-length
//	    Print the length of the longest line.
//
//	--help
//	    Print this usage message to stdout and exit 0.
//
// Output columns always appear in a fixed order: lines, words, chars,
// bytes, max-line-length. Only the requested columns are shown. Column
// widths are right-justified to the width of the largest count across
// all files (including the total line, if any).
//
// Exit codes:
//
//	0  All files processed successfully.
//	1  At least one error occurred (missing file, invalid argument, etc.).
//
// Memory safety:
//
//	Input is read in fixed-size chunks (32 KiB). Lines longer than
//	MaxLineBytes (1 MiB) are split across chunks for counting purposes
//	but never fully buffered. All loops check ctx.Err() at each
//	iteration to honour the shell's execution timeout.
package wc

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the wc builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "wc",
	Description: "print newline, word, and byte counts",
	MakeFlags:   registerFlags,
}

const chunkSize = 32 * 1024  // 32 KiB read buffer
const nonRegularMinWidth = 7 // GNU wc minimum column width for non-regular files

type counts struct {
	lines      int64
	words      int64
	chars      int64
	bytes      int64
	maxLineLen int64
}

type options struct {
	showLines      bool
	showWords      bool
	showBytes      bool
	showChars      bool
	showMaxLineLen bool
}

// numCols returns the number of output columns that will be printed.
func (o options) numCols() int {
	n := 0
	if o.showLines {
		n++
	}
	if o.showWords {
		n++
	}
	if o.showChars {
		n++
	}
	if o.showBytes {
		n++
	}
	if o.showMaxLineLen {
		n++
	}
	return n
}

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.Bool("help", false, "print usage and exit")
	lines := fs.BoolP("lines", "l", false, "print the newline counts")
	words := fs.BoolP("words", "w", false, "print the word counts")
	bytesFlag := fs.BoolP("bytes", "c", false, "print the byte counts")
	chars := fs.BoolP("chars", "m", false, "print the character counts")
	maxLineLen := fs.BoolP("max-line-length", "L", false, "print the maximum display width")

	// Security: --files0-from is intentionally NOT implemented.
	// GTFOBins: this flag reads filenames from a file, enabling
	// data exfiltration in sandboxed environments.

	return func(ctx context.Context, callCtx *builtins.CallContext, files []string) builtins.Result {
		if *help {
			callCtx.Out("Usage: wc [OPTION]... [FILE]...\n")
			callCtx.Out("Print newline, word, and byte counts for each FILE.\n")
			callCtx.Out("With no FILE, or when FILE is -, read standard input.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		opts := options{
			showLines:      *lines,
			showWords:      *words,
			showBytes:      *bytesFlag,
			showChars:      *chars,
			showMaxLineLen: *maxLineLen,
		}

		if !opts.showLines && !opts.showWords && !opts.showBytes && !opts.showChars && !opts.showMaxLineLen {
			opts.showLines = true
			opts.showWords = true
			opts.showBytes = true
		}

		stdinImplicit := len(files) == 0
		if stdinImplicit {
			files = []string{"-"}
		}

		hasStdin := stdinImplicit
		if !hasStdin {
			for _, f := range files {
				if f == "-" {
					hasStdin = true
					break
				}
			}
		}

		var total counts
		var failed bool

		type fileResult struct {
			name string
			c    counts
		}
		results := make([]fileResult, 0, len(files))
		hasNonRegular := hasStdin // stdin (pipe) is non-regular

		for _, file := range files {
			if ctx.Err() != nil {
				break
			}
			c, err := countFile(ctx, callCtx, file)
			if err != nil {
				name := file
				if file == "-" {
					name = "standard input"
				}
				if name == "" {
					callCtx.Errf("wc: %s\n", callCtx.PortableErr(err))
				} else {
					callCtx.Errf("wc: %s: %s\n", name, callCtx.PortableErr(err))
				}
				failed = true
				// GNU wc prints a zero count line for directories but not
				// for missing files or other open errors.
				if !isErrIsDir(err) {
					continue
				}
				hasNonRegular = true
			}
			results = append(results, fileResult{name: file, c: c})
			total.lines += c.lines
			total.words += c.words
			total.chars += c.chars
			total.bytes += c.bytes
			if c.maxLineLen > total.maxLineLen {
				total.maxLineLen = c.maxLineLen
			}
		}

		width := fieldWidth(total, opts)
		// GNU wc uses a minimum column width of 7 for non-regular files
		// (stdin pipes, directories, devices, etc.) when two or more
		// columns are printed or when multiple files are processed.
		// With a single column and single file, no minimum is applied.
		if hasNonRegular && (opts.numCols() >= 2 || len(files) > 1) && width < nonRegularMinWidth {
			width = nonRegularMinWidth
		}

		for _, fr := range results {
			name := fr.name
			if name == "-" && stdinImplicit {
				name = ""
			}
			printCounts(callCtx, fr.c, opts, width, name)
		}

		if len(files) > 1 {
			printCounts(callCtx, total, opts, width, "total")
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

func countFile(ctx context.Context, callCtx *builtins.CallContext, path string) (counts, error) {
	if path == "" {
		return counts{}, errors.New("invalid zero-length file name")
	}
	var rc io.ReadCloser
	if path == "-" {
		if callCtx.Stdin == nil {
			return counts{}, nil
		}
		rc = io.NopCloser(callCtx.Stdin)
	} else {
		f, err := callCtx.OpenFile(ctx, path, os.O_RDONLY, 0)
		if err != nil {
			return counts{}, err
		}
		rc = f
	}
	defer rc.Close()
	return countReader(ctx, rc)
}

func countReader(ctx context.Context, r io.Reader) (counts, error) {
	buf := make([]byte, chunkSize)
	var c counts
	var inWord bool
	var lineLen int64
	var carry [utf8.UTFMax - 1]byte
	var carryN int

	for {
		if ctx.Err() != nil {
			return c, ctx.Err()
		}
		n, err := r.Read(buf[carryN:])
		if carryN > 0 {
			copy(buf, carry[:carryN])
			n += carryN
			carryN = 0
		}
		if n > 0 {
			chunk := buf[:n]
			c.bytes += int64(n)

			// Handle incomplete UTF-8 at end of chunk.
			// When tail >= n (e.g., n == 1 with a single invalid byte), the
			// condition below is false, so the byte stays in chunk and
			// DecodeRune processes it as a replacement character — this is
			// correct and matches utf8.DecodeRune semantics.
			tail := 0
			if !utf8.Valid(chunk) {
				for tail = 1; tail <= 3 && tail < n; tail++ {
					if utf8.Valid(chunk[:n-tail]) {
						break
					}
				}
				if tail > 0 && tail <= 3 && tail < n {
					carryN = copy(carry[:], chunk[n-tail:])
					chunk = chunk[:n-tail]
				} else {
					tail = 0
				}
			}
			// carryN bytes are subtracted here and will be re-added via
			// n += carryN at the top of the next iteration.
			c.bytes -= int64(carryN)

			for i := 0; i < len(chunk); {
				ch, size := utf8.DecodeRune(chunk[i:])
				i += size
				// Invalid UTF-8 byte: not a character in C.UTF-8 locale.
				// Skip entirely — no char count, no word effect.
				if ch == utf8.RuneError && size == 1 {
					continue
				}
				c.chars++
				if ch == '\n' {
					c.lines++
					if lineLen > c.maxLineLen {
						c.maxLineLen = lineLen
					}
					lineLen = 0
					inWord = false
				} else if ch == '\r' {
					if lineLen > c.maxLineLen {
						c.maxLineLen = lineLen
					}
					lineLen = 0
					inWord = false
				} else if ch == '\t' {
					lineLen = (lineLen/8 + 1) * 8
					inWord = false
				} else if ch == ' ' {
					lineLen++
					inWord = false
				} else if ch == '\f' {
					if lineLen > c.maxLineLen {
						c.maxLineLen = lineLen
					}
					lineLen = 0
					inWord = false
				} else if ch == '\v' {
					// vertical tab: zero display width, but breaks words
					inWord = false
				} else if unicode.Is(unicode.Cc, ch) {
					// Control characters are transparent to word counting:
					// they don't start or end words, matching GNU wc.
					lineLen += int64(runeWidth(ch))
				} else if unicode.Is(unicode.Zs, ch) {
					// Unicode space separators (NBSP, thin space, etc.) end words,
					// matching GNU wc behaviour under C.UTF-8 locale.
					lineLen++
					inWord = false
				} else if unicode.IsGraphic(ch) || unicode.Is(unicode.Co, ch) || unicode.Is(unicode.Cf, ch) || unicode.Is(unicode151Print, ch) {
					// Printable characters start or continue a word,
					// matching GNU wc which gates word counting on
					// iswprint() in C.UTF-8 locale. IsGraphic covers
					// letters, marks, numbers, punctuation, and
					// symbols; Co adds private-use characters; Cf adds
					// format characters (e.g. U+06DD ARABIC END OF
					// AYAH, U+200B ZERO WIDTH SPACE) which glibc's
					// iswprint considers printable; unicode151Print
					// adds characters assigned in Unicode 15.1 that
					// Go's tables don't yet include (Go ships
					// Unicode 15.0).
					if !inWord {
						c.words++
						inWord = true
					}
					lineLen += int64(runeWidth(ch))
				}
				// Other non-printable characters do not affect words or line length.
			}
		}
		if err == io.EOF {
			if carryN > 0 {
				// Incomplete UTF-8 sequence at EOF: counts as bytes but not chars.
				c.bytes += int64(carryN)
			}
			break
		}
		if err != nil {
			return c, err
		}
	}
	if lineLen > c.maxLineLen {
		c.maxLineLen = lineLen
	}
	return c, nil
}

func fieldWidth(total counts, opts options) int {
	// GNU wc behaviour:
	//   - When multiple columns are displayed, the field width is derived
	//     from the largest count across ALL categories (lines, words,
	//     chars, bytes) — even hidden ones. This keeps columns aligned
	//     when only a subset is printed (e.g. "wc -lw").
	//   - When only a single column is displayed, the field width equals
	//     that column's own digit count (no padding from hidden counters).
	//   - maxLineLen is only considered when -L is explicitly requested.
	numCols := 0
	if opts.showLines {
		numCols++
	}
	if opts.showWords {
		numCols++
	}
	if opts.showChars {
		numCols++
	}
	if opts.showBytes {
		numCols++
	}
	if opts.showMaxLineLen {
		numCols++
	}

	var max int64
	if numCols > 1 {
		// Multi-column: use all standard categories for alignment.
		max = total.lines
		if total.words > max {
			max = total.words
		}
		if total.chars > max {
			max = total.chars
		}
		if total.bytes > max {
			max = total.bytes
		}
		if opts.showMaxLineLen && total.maxLineLen > max {
			max = total.maxLineLen
		}
	} else {
		// Single column: width is just that column's value.
		switch {
		case opts.showLines:
			max = total.lines
		case opts.showWords:
			max = total.words
		case opts.showChars:
			max = total.chars
		case opts.showBytes:
			max = total.bytes
		case opts.showMaxLineLen:
			max = total.maxLineLen
		}
	}
	w := len(strconv.FormatInt(max, 10))
	return w
}

// unicode151Print covers characters assigned in Unicode 15.1 that are
// printable (graphic) but absent from Go's unicode package (Unicode 15.0).
// CI runs GNU wc linked against glibc ≥ 2.39 (Ubuntu 24.04) which uses
// Unicode 15.1+ character data, so these codepoints must be treated as
// word characters to match GNU wc output.
//
// This table can be removed once Go's unicode package is updated to
// Unicode 15.1 or later (tracked in https://github.com/golang/go/issues/65141,
// expected in Go 1.27).
var unicode151Print = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x2FFC, 0x2FFF, 1}, // Ideographic Description Characters (4 new IDCs)
		{0x31EF, 0x31EF, 1}, // Ideographic Description Character OVERLAID
	},
	R32: []unicode.Range32{
		{0x2EBF0, 0x2EE5D, 1}, // CJK Unified Ideographs Extension I
	},
}

// runeWidth returns the display width of a rune following wcwidth(3) rules:
// 0 for controls, combining marks, and format chars; 2 for East Asian
// Wide/Fullwidth; 1 for everything else.
func runeWidth(r rune) int {
	if unicode.Is(unicode.Cc, r) {
		return 0
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	// Hangul Jamo medial vowels and final consonants (zero-width in syllable composition).
	if r >= 0x1160 && r <= 0x11FF {
		return 0
	}
	if unicode.Is(eastAsianWide, r) {
		return 2
	}
	return 1
}

// eastAsianWide is a RangeTable covering East Asian Wide and Fullwidth
// codepoints per UAX #11, matching the ranges used by wcwidth(3).
var eastAsianWide = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x1100, 0x115F, 1}, // Hangul Jamo initials
		{0x2329, 0x232A, 1}, // CJK angle brackets
		{0x2E80, 0x303E, 1}, // CJK Radicals Supplement .. CJK Symbols
		{0x3040, 0x33BF, 1}, // Hiragana .. CJK Compatibility
		{0x33C0, 0x33FF, 1}, // CJK Compatibility (cont.)
		{0x3400, 0x4DBF, 1}, // CJK Unified Ideographs Extension A
		{0x4E00, 0xA4CF, 1}, // CJK Unified Ideographs .. Yi
		{0xAC00, 0xD7A3, 1}, // Hangul Syllables
		{0xF900, 0xFAFF, 1}, // CJK Compatibility Ideographs
		{0xFE10, 0xFE19, 1}, // Vertical Forms
		{0xFE30, 0xFE6F, 1}, // CJK Compatibility Forms + Small Form Variants
		{0xFF01, 0xFF60, 1}, // Fullwidth Forms
		{0xFFE0, 0xFFE6, 1}, // Fullwidth Signs
	},
	R32: []unicode.Range32{
		{0x1F300, 0x1F64F, 1}, // Misc Symbols/Pictographs + Emoticons
		{0x1F900, 0x1F9FF, 1}, // Supplemental Symbols and Pictographs
		{0x20000, 0x2FFFD, 1}, // CJK Extension B..F
		{0x30000, 0x3FFFD, 1}, // CJK Extension G+
	},
}

func printCounts(callCtx *builtins.CallContext, c counts, opts options, width int, name string) {
	first := true
	printField := func(val int64) {
		if first {
			callCtx.Outf("%*d", width, val)
			first = false
		} else {
			callCtx.Outf(" %*d", width, val)
		}
	}
	if opts.showLines {
		printField(c.lines)
	}
	if opts.showWords {
		printField(c.words)
	}
	if opts.showChars {
		printField(c.chars)
	}
	if opts.showBytes {
		printField(c.bytes)
	}
	if opts.showMaxLineLen {
		printField(c.maxLineLen)
	}
	if name != "" {
		callCtx.Outf(" %s", name)
	}
	callCtx.Out("\n")
}
