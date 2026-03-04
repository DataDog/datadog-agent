// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package interp implements a safe, restricted POSIX shell interpreter.
//
// builtinTail implements the tail command. Accepted flags:
//
//	-n N / --lines=N        Last N lines (default 10). +N = from line N to end.
//	-c N / --bytes=N        Last N bytes. +N = from byte N to end.
//	-q / --quiet / --silent Never print headers for multiple files.
//	-v / --verbose          Always print headers (even for a single file).
//	-z / --zero-terminated  Use NUL as line delimiter.
//	-h / --help             Print usage information and exit.
//
// Any flag not listed above is rejected by the pflag parser with an
// "unknown flag" error written to stderr and exit code 1.
//
// # Line ending behaviour
//
// Line mode uses bufio.ScanLines, which recognises LF (\n) and CRLF (\r\n) as
// line terminators.  The trailing \r is stripped from each token, so CRLF input
// is normalised to LF on output.  CR-only files (\r with no \n) have no
// recognised terminator; the entire content is treated as a single line with
// embedded \r bytes preserved.  Byte mode (-c) is a raw copy and preserves all
// byte values unchanged.
package interp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

const (
	maxTailLines = 10_000           // max ring-buffer capacity (lines)
	maxLineBytes = 1 * 1024 * 1024  // 1 MB per-line scanner cap
	maxTailBytes = 10 * 1024 * 1024 // 10 MB ring buffer for byte mode
)

// tailMode selects which output algorithm tailOutput uses.
type tailMode int

const (
	tailModeLastLines tailMode = iota // last N lines (default)
	tailModeLastBytes                 // last N bytes (-c N)
	tailModeFromLine                  // lines from line N to EOF (-n +N)
	tailModeFromByte                  // bytes from byte N to EOF (-c +N)
)

func (r *Runner) builtinTail(ctx context.Context, args []string) error {
	// Build a pflag FlagSet.  ContinueOnError makes Parse return an error
	// instead of calling os.Exit.  SetOutput(io.Discard) suppresses pflag's
	// own error printing so we can format errors ourselves.
	fs := pflag.NewFlagSet("tail", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)

	// -n and -c are registered as strings so we can handle the +N offset
	// prefix ourselves after pflag has done the structural parsing.
	linesStr := fs.StringP("lines", "n", "", "output the last N lines (default 10); +N outputs starting at line N")
	bytesStr := fs.StringP("bytes", "c", "", "output the last N bytes; +N outputs starting at byte N")
	quiet := fs.BoolP("quiet", "q", false, "never print file headers")
	silent := fs.Bool("silent", false, "alias for --quiet")
	verbose := fs.BoolP("verbose", "v", false, "always print file headers")
	zeroDelim := fs.BoolP("zero-terminated", "z", false, "use NUL as line delimiter instead of newline")
	help := fs.BoolP("help", "h", false, "display this help and exit")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(r.stderr, "tail: %v\n", err)
		r.exitCode = 1
		return nil
	}

	if *help {
		fmt.Fprintf(r.stdout,
			"Usage: tail [OPTION]... [FILE]...\n\n"+
				"Print the last 10 lines of each FILE to standard output.\n"+
				"With no FILE, or when FILE is -, read standard input.\n\n"+
				"Options:\n")
		fs.SetOutput(r.stdout)
		fs.PrintDefaults()
		r.exitCode = 0
		return nil
	}

	files := fs.Args()

	// Determine the output mode and its count/offset value n.
	// If both -n and -c are given, -c (bytes) takes precedence.
	mode := tailModeLastLines
	n := 10 // default: last 10 lines

	if fs.Changed("bytes") {
		var count, offset int = -1, -1
		if err := parseTailOffset(*bytesStr, "byte", &count, &offset); err != nil {
			fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", *bytesStr)
			r.exitCode = 1
			return nil
		}
		if offset >= 0 {
			mode, n = tailModeFromByte, offset
		} else {
			mode, n = tailModeLastBytes, count
		}
	} else if fs.Changed("lines") {
		var count, offset int = -1, -1
		if err := parseTailOffset(*linesStr, "line", &count, &offset); err != nil {
			fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", *linesStr)
			r.exitCode = 1
			return nil
		}
		if offset >= 0 {
			mode, n = tailModeFromLine, offset
		} else {
			mode, n = tailModeLastLines, count
		}
	}

	quietFlag := *quiet || *silent
	verboseFlag := *verbose

	// lineSep is the delimiter byte used for both line-splitting and output.
	lineSep := byte('\n')
	if *zeroDelim {
		lineSep = 0
	}

	// tailOutput processes a single reader and writes its tail to r.stdout.
	tailOutput := func(reader io.Reader, name string, showHeader bool) error {
		if showHeader {
			fmt.Fprintf(r.stdout, "==> %s <==\n", name)
		}

		switch mode {
		case tailModeFromByte:
			// +N byte offset: skip first N-1 bytes, copy rest.
			toSkip := max(int64(n-1), 0)
			if _, err := io.CopyN(io.Discard, reader, toSkip); err != nil && err != io.EOF {
				return err
			}
			_, err := io.Copy(r.stdout, reader)
			return err

		case tailModeLastBytes:
			// Last N bytes.
			// Try seek optimisation for regular files.
			if f, ok := reader.(*os.File); ok {
				if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
					size := info.Size()
					offset := max(size-int64(n), 0)
					if _, err := f.Seek(offset, io.SeekStart); err == nil {
						_, err = io.Copy(r.stdout, f)
						return err
					}
				}
			}
			// Non-seekable: circular byte buffer.
			// Check ctx before allocating to avoid wasted work on an already-cancelled context.
			if err := ctx.Err(); err != nil {
				return err
			}
			bufSize := min(n, maxTailBytes)
			if bufSize == 0 {
				return nil
			}
			buf := make([]byte, bufSize)
			head := 0   // next write position
			filled := 0 // how many bytes are valid
			tmp := make([]byte, 32*1024)
			for {
				// Honour context cancellation (e.g., executor timeout) before
				// each read so that infinite sources like /dev/zero terminate
				// promptly rather than spinning until the OS kills the process.
				if err := ctx.Err(); err != nil {
					break
				}
				nr, err := reader.Read(tmp)
				for pos := 0; pos < nr; {
					space := bufSize - head
					toCopy := min(nr-pos, space)
					copy(buf[head:head+toCopy], tmp[pos:pos+toCopy])
					head = (head + toCopy) % bufSize
					pos += toCopy
					if filled < bufSize {
						filled = min(filled+toCopy, bufSize)
					}
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}
			}
			if filled == 0 {
				return nil
			}
			if filled < bufSize {
				// Buffer not yet full; data starts at 0.
				_, err := r.stdout.Write(buf[:filled])
				return err
			}
			// Full ring: oldest data starts at head.
			if _, err := r.stdout.Write(buf[head:]); err != nil {
				return err
			}
			if head > 0 {
				_, err := r.stdout.Write(buf[:head])
				return err
			}
			return nil

		case tailModeFromLine:
			// +N: from line N to end.
			toSkip := max(n-1, 0)
			scanner := newTailScanner(reader, lineSep)
			skipped := 0
			for scanner.Scan() {
				if err := ctx.Err(); err != nil {
					return err
				}
				if skipped < toSkip {
					skipped++
					continue
				}
				if _, err := r.stdout.Write(scanner.Bytes()); err != nil {
					return err
				}
				if _, err := r.stdout.Write([]byte{lineSep}); err != nil {
					return err
				}
			}
			return scanner.Err()

		default: // tailModeLastLines
			// Last N lines: ring buffer of strings.
			size := max(n, 0)
			size = min(size, maxTailLines)
			if size == 0 {
				// Drain input, output nothing.
				_, err := io.Copy(io.Discard, reader)
				return err
			}
			ring := make([]string, size)
			head := 0
			seen := 0
			scanner := newTailScanner(reader, lineSep)
			for scanner.Scan() {
				if err := ctx.Err(); err != nil {
					return err
				}
				ring[head] = scanner.Text()
				head = (head + 1) % size
				seen++
			}
			if err := scanner.Err(); err != nil {
				return err
			}
			// Determine start position and how many lines to print.
			total := min(seen, size)
			start := head - total
			if start < 0 {
				start += size
			}
			for i := range total {
				idx := (start + i) % size
				if _, err := io.WriteString(r.stdout, ring[idx]); err != nil {
					return err
				}
				if _, err := r.stdout.Write([]byte{lineSep}); err != nil {
					return err
				}
			}
			return nil
		}
	}

	showHeader := (len(files) > 1 || verboseFlag) && !quietFlag

	if len(files) == 0 {
		if err := tailOutput(r.stdin, "", false); err != nil {
			fmt.Fprintf(r.stderr, "tail: %v\n", err)
			r.exitCode = 1
			return nil
		}
		r.exitCode = 0
		return nil
	}

	hasError := false
	for idx, f := range files {
		if idx > 0 && showHeader {
			fmt.Fprintln(r.stdout)
		}
		if f == "-" {
			if err := tailOutput(r.stdin, "standard input", showHeader); err != nil {
				fmt.Fprintf(r.stderr, "tail: standard input: %v\n", err)
				hasError = true
			}
			continue
		}
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.dir, path)
		}
		if tailIsWindowsReservedName(f) {
			fmt.Fprintf(r.stderr, "tail: cannot open %q for reading: reserved device name\n", f)
			hasError = true
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(r.stderr, "tail: cannot open %q for reading: %v\n", f, err)
			hasError = true
			continue
		}
		// IIFE scopes defer file.Close() to this iteration so the file
		// descriptor is released immediately rather than at function return.
		if err := func() error {
			defer file.Close()
			return tailOutput(file, f, showHeader)
		}(); err != nil {
			fmt.Fprintf(r.stderr, "tail: %s: %v\n", f, err)
			hasError = true
		}
	}
	if hasError {
		r.exitCode = 1
	} else {
		r.exitCode = 0
	}
	return nil
}

// parseTailOffset parses a -n or -c argument value.  If the value starts with
// '+', it sets *offset and clears *count (offset mode).  Otherwise it sets
// *count and clears *offset (last-N mode).  Negative values are rejected.
// kind ("line" or "byte") is used only in error messages.
func parseTailOffset(raw, kind string, count, offset *int) error {
	if strings.HasPrefix(raw, "+") {
		n, err := strconv.Atoi(raw[1:])
		if err != nil {
			return err
		}
		if n < 0 {
			return fmt.Errorf("invalid negative %s offset", kind)
		}
		*offset = n
		*count = -1
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return err
	}
	if n < 0 {
		return fmt.Errorf("invalid negative %s count", kind)
	}
	*count = n
	*offset = -1
	return nil
}

// newTailScanner returns a bufio.Scanner with the given delimiter and a 1MB buffer.
func newTailScanner(r io.Reader, delim byte) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, maxLineBytes)
	scanner.Buffer(buf, maxLineBytes)
	if delim == 0 {
		scanner.Split(scanNULLines)
	}
	return scanner
}

// scanNULLines is a bufio.SplitFunc that splits on NUL bytes instead of newlines.
func scanNULLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i := range len(data) {
		if data[i] == 0 {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// tailIsWindowsReservedName reports whether name is a Windows reserved device
// name (CON, NUL, COM1-9, LPT1-9, etc.).  Opening these names on Windows can
// cause hangs or security issues.  On non-Windows platforms this always returns
// false so files legitimately named "NUL" are accessible.
func tailIsWindowsReservedName(name string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	base := strings.ToUpper(filepath.Base(name))
	// Strip extension: "NUL.txt" is still reserved on Windows.
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	switch base {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5",
		"COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5",
		"LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}
	return false
}
