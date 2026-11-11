// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package cat implements the cat builtin command.
//
// cat — concatenate and print files
//
// Usage: cat [OPTION]... [FILE]...
//
// Concatenate FILE(s) to standard output.
// With no FILE, or when FILE is -, read standard input.
//
// Accepted flags:
//
//	-n, --number
//	    Number all output lines, starting at 1. Line numbers are
//	    right-justified in a 6-character field followed by a tab.
//
//	-b, --number-nonblank
//	    Number only non-blank output lines, starting at 1. Overrides -n.
//
//	-s, --squeeze-blank
//	    Squeeze multiple consecutive blank lines into a single blank line.
//
//	-E, --show-ends
//	    Display a $ character at the end of each line.
//
//	-T, --show-tabs
//	    Display TAB characters as ^I.
//
//	-v, --show-nonprinting
//	    Display non-printing characters using ^ and M- notation, except
//	    for line-feed and TAB.
//
//	-A, --show-all
//	    Equivalent to -vET.
//
//	-e
//	    Equivalent to -vE.
//
//	-t
//	    Equivalent to -vT.
//
//	-u
//	    Ignored (output is already unbuffered). Accepted for POSIX
//	    compatibility.
//
//	--help
//	    Print usage to stdout and exit 0.
//
// Exit codes:
//
//	0  All files processed successfully.
//	1  At least one error occurred (missing file, permission denied, etc.).
//
// Memory safety:
//
//	When line-processing flags are active (-n, -b, -s, -E, -T, -v), input
//	is read line-by-line with a per-line cap of MaxLineBytes (1 MiB). Lines
//	exceeding this cap cause an error rather than an unbounded allocation.
//	Without flags, input is streamed in fixed-size chunks (no per-line cap).
//	All read loops check ctx.Err() at each iteration to honour the shell's
//	execution timeout and support graceful cancellation.
package cat

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the cat builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "cat",
	Description: "concatenate and print files",
	MakeFlags:   registerFlags,
}

// MaxLineBytes is the per-line buffer cap for the line scanner. Lines
// longer than this are reported as an error instead of being buffered.
const MaxLineBytes = 1 << 20 // 1 MiB

const (
	rawBufSize   = 32 * 1024 // read buffer for catRaw
	scanBufInit  = 4096      // initial scanner buffer
	lineBufInit  = 4096      // initial output-line buffer
	lineNumWidth = 6         // GNU cat line-number field width
)

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.Bool("help", false, "print usage and exit")
	number := fs.BoolP("number", "n", false, "number all output lines")
	numberNonblank := fs.BoolP("number-nonblank", "b", false, "number non-blank output lines, overrides -n")
	squeezeBlank := fs.BoolP("squeeze-blank", "s", false, "suppress repeated empty output lines")
	showEnds := fs.BoolP("show-ends", "E", false, "display $ at end of each line")
	showTabs := fs.BoolP("show-tabs", "T", false, "display TAB characters as ^I")
	showNonprinting := fs.BoolP("show-nonprinting", "v", false, "use ^ and M- notation, except for LFD and TAB")
	showAll := fs.BoolP("show-all", "A", false, "equivalent to -vET")
	flagE := fs.BoolP("show-nonprinting-ends", "e", false, "equivalent to -vE")
	flagT := fs.BoolP("show-nonprinting-tabs", "t", false, "equivalent to -vT")
	_ = fs.BoolP("unbuffered", "u", false, "ignored")

	return func(ctx context.Context, callCtx *builtins.CallContext, files []string) builtins.Result {
		if *help {
			callCtx.Out("Usage: cat [OPTION]... [FILE]...\n")
			callCtx.Out("Concatenate FILE(s) to standard output.\n")
			callCtx.Out("With no FILE, or when FILE is -, read standard input.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		if *showAll {
			*showNonprinting = true
			*showEnds = true
			*showTabs = true
		}
		if *flagE {
			*showNonprinting = true
			*showEnds = true
		}
		if *flagT {
			*showNonprinting = true
			*showTabs = true
		}
		if *numberNonblank {
			*number = false
		}

		needsLineProcessing := *number || *numberNonblank || *squeezeBlank ||
			*showEnds || *showTabs || *showNonprinting

		if len(files) == 0 {
			files = []string{"-"}
		}

		st := &state{
			number:          *number,
			numberNonblank:  *numberNonblank,
			squeezeBlank:    *squeezeBlank,
			showEnds:        *showEnds,
			showTabs:        *showTabs,
			showNonprinting: *showNonprinting,
			lineNum:         1,
		}

		var failed bool
		for _, file := range files {
			if ctx.Err() != nil {
				break
			}
			var err error
			if needsLineProcessing {
				err = catLines(ctx, callCtx, file, st)
			} else {
				err = catRaw(ctx, callCtx, file)
			}
			if err != nil {
				if builtins.IsBrokenPipe(err) {
					break
				}
				name := file
				if file == "-" {
					name = "standard input"
				}
				callCtx.Errf("cat: %s: %s\n", name, callCtx.PortableErr(err))
				failed = true
			}
		}

		if failed {
			return builtins.Result{Code: 1}
		}
		return builtins.Result{}
	}
}

type state struct {
	number          bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showTabs        bool
	showNonprinting bool
	lineNum         int
	prevBlank       bool
}

func openReader(ctx context.Context, callCtx *builtins.CallContext, file string) (io.ReadCloser, error) {
	if file == "-" {
		if callCtx.Stdin == nil {
			return nil, nil
		}
		return io.NopCloser(callCtx.Stdin), nil
	}
	return callCtx.OpenFile(ctx, file, os.O_RDONLY, 0)
}

// catRaw streams a file to stdout with bounded reads and context checking.
// Used when no line-processing flags are active.
func catRaw(ctx context.Context, callCtx *builtins.CallContext, file string) error {
	rc, err := openReader(ctx, callCtx, file)
	if err != nil {
		return err
	}
	if rc == nil {
		return nil
	}
	defer rc.Close()

	buf := make([]byte, rawBufSize)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, readErr := rc.Read(buf)
		if n > 0 {
			if _, werr := callCtx.Stdout.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

// catLines reads a file line-by-line, applying display transformations.
func catLines(ctx context.Context, callCtx *builtins.CallContext, file string, st *state) error {
	rc, err := openReader(ctx, callCtx, file)
	if err != nil {
		return err
	}
	if rc == nil {
		return nil
	}
	defer rc.Close()

	sc := bufio.NewScanner(rc)
	buf := make([]byte, scanBufInit)
	sc.Buffer(buf, MaxLineBytes+1)
	sc.Split(scanLinesPreservingNewline)

	out := make([]byte, 0, lineBufInit)

	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := sc.Bytes()
		content, term := splitTerminator(line)
		hasTerm := len(term) > 0
		blank := len(content) == 0 && hasTerm

		if st.squeezeBlank && blank && st.prevBlank {
			continue
		}
		st.prevBlank = blank

		out = out[:0]

		if (st.numberNonblank && !blank) || st.number {
			out = appendNumber(out, st.lineNum)
			st.lineNum++
		}

		// GNU cat -E converts \r immediately before \n to ^M to prevent
		// the terminal cursor from overwriting the $ marker.  When -v is
		// active the \r is already converted by appendNonprinting.
		hasCRLF := st.showEnds && !st.showNonprinting &&
			len(content) > 0 && content[len(content)-1] == '\r' && hasTerm

		processLen := len(content)
		if hasCRLF {
			processLen--
		}

		for _, b := range content[:processLen] {
			if st.showTabs && b == '\t' {
				out = append(out, '^', 'I')
			} else if st.showNonprinting {
				out = appendNonprinting(out, b)
			} else {
				out = append(out, b)
			}
		}

		if hasCRLF {
			out = append(out, '^', 'M')
		}

		if st.showEnds && hasTerm {
			out = append(out, '$')
		}

		out = append(out, term...)

		if _, werr := callCtx.Stdout.Write(out); werr != nil {
			return werr
		}
	}
	return sc.Err()
}

// splitTerminator separates a scanner token into the content portion and
// the line terminator (\n), if present.
func splitTerminator(line []byte) (content, term []byte) {
	n := len(line)
	if n > 0 && line[n-1] == '\n' {
		return line[:n-1], line[n-1:]
	}
	return line, nil
}

// appendNonprinting encodes a single byte in ^ and M- notation.
// TAB and LF pass through unchanged (they have their own flags).
func appendNonprinting(out []byte, b byte) []byte {
	switch {
	case b == '\t':
		return append(out, '\t')
	case b == '\n':
		return append(out, '\n')
	case b < 32:
		return append(out, '^', b+64)
	case b < 127:
		return append(out, b)
	case b == 127:
		return append(out, '^', '?')
	case b < 128+32:
		return append(out, 'M', '-', '^', b-128+64)
	case b < 128+127:
		return append(out, 'M', '-', b-128)
	default: // 255
		return append(out, 'M', '-', '^', '?')
	}
}

// appendNumber formats n as a right-justified field of lineNumWidth
// characters followed by a tab, matching the GNU cat line-number format.
func appendNumber(out []byte, n int) []byte {
	var digits [20]byte
	pos := len(digits)
	v := n
	if v <= 0 {
		pos--
		digits[pos] = '0'
	}
	for v > 0 {
		pos--
		digits[pos] = byte('0' + v%10)
		v /= 10
	}
	for i := len(digits) - pos; i < lineNumWidth; i++ {
		out = append(out, ' ')
	}
	out = append(out, digits[pos:]...)
	return append(out, '\t')
}

// scanLinesPreservingNewline is a bufio.SplitFunc that includes the line
// terminator (\n) in the returned token. Unlike bufio.ScanLines, it does not
// strip \r\n or \n, so the caller reproduces the exact file content.
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
