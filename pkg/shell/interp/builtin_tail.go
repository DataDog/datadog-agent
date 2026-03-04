// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package interp implements a safe, restricted POSIX shell interpreter.
//
// builtinTail implements the tail command. Accepted flags:
//
//	-n N / --lines=N   Last N lines (default 10). +N = from line N to end.
//	-c N / --bytes=N   Last N bytes. +N = from byte N to end.
//	-q / --quiet       Never print headers for multiple files.
//	-v / --verbose     Always print headers (even for a single file).
//	-z / --zero-terminated  Use NUL as line delimiter.
//
// Rejected flags (cause infinite blocking loops):
//
//	-f / --follow / -F / --retry / -s / --sleep-interval / --pid / --max-unchanged-stats
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

func (r *Runner) builtinTail(ctx context.Context, args []string) error {
	// Pre-scan for follow-mode flags before pflag sees anything.  These must
	// be hard-rejected with a specific error because they block forever.
	// Stop scanning at "--" (end-of-flags marker).
	for _, a := range args {
		if a == "--" {
			break
		}
		switch {
		case a == "-f", a == "--follow", a == "-F", a == "--retry",
			a == "--pid", strings.HasPrefix(a, "--pid="),
			a == "--max-unchanged-stats", strings.HasPrefix(a, "--max-unchanged-stats="):
			return fmt.Errorf("flag %q is not allowed for command \"tail\": follow/retry flags cause infinite blocking loops", a)
		case strings.HasPrefix(a, "-s"), a == "--sleep-interval",
			strings.HasPrefix(a, "--sleep-interval="):
			return fmt.Errorf("flag %q is not allowed for command \"tail\": follow/retry flags cause infinite blocking loops", a)
		}
	}

	// Build a pflag FlagSet.  ContinueOnError makes Parse return an error
	// instead of calling os.Exit.  SetOutput(io.Discard) suppresses pflag's
	// own error printing so we can format errors ourselves.
	fs := pflag.NewFlagSet("tail", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)

	// -n and -c are registered as strings so we can handle the +N offset
	// prefix ourselves after pflag has done the structural parsing.
	linesStr := fs.StringP("lines", "n", "", "output the last N lines (default 10); +N outputs from line N")
	bytesStr := fs.StringP("bytes", "c", "", "output the last N bytes; +N outputs from byte N")
	quiet := fs.BoolP("quiet", "q", false, "never print file headers")
	silent := fs.Bool("silent", false, "alias for --quiet")
	verbose := fs.BoolP("verbose", "v", false, "always print file headers")
	zeroDelim := fs.BoolP("zero-terminated", "z", false, "use NUL as line delimiter")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(r.stderr, "tail: %v\n", err)
		r.exitCode = 1
		return nil
	}

	files := fs.Args()

	// Determine mode.  If both -n and -c are given, -c (bytes) takes
	// precedence — consistent with most tail implementations where the last
	// positional mode flag wins and byte mode is more specific.
	lineCount := 10  // default
	byteCount := -1
	lineOffset := -1
	byteOffset := -1

	if fs.Changed("bytes") {
		lineCount = -1
		lineOffset = -1
		if err := parseTailC(*bytesStr, &byteCount, &byteOffset); err != nil {
			fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", *bytesStr)
			r.exitCode = 1
			return nil
		}
	} else if fs.Changed("lines") {
		byteCount = -1
		byteOffset = -1
		if err := parseTailN(*linesStr, &lineCount, &lineOffset); err != nil {
			fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", *linesStr)
			r.exitCode = 1
			return nil
		}
	}
	// else: neither flag given — defaults (lineCount=10) stand.

	quietFlag := *quiet || *silent
	verboseFlag := *verbose
	zeroDelimFlag := *zeroDelim

	// tailOutput processes a single reader and writes its tail to stdout.
	tailOutput := func(reader io.Reader, name string, showHeader bool) error {
		if showHeader {
			fmt.Fprintf(r.stdout, "==> %s <==\n", name)
		}

		if byteOffset >= 0 {
			// +N byte offset: skip first N-1 bytes, copy rest.
			toSkip := max(int64(byteOffset-1), 0)
			if _, err := io.CopyN(io.Discard, reader, toSkip); err != nil && err != io.EOF {
				return err
			}
			_, err := io.Copy(r.stdout, reader)
			return err
		}

		if byteCount >= 0 {
			// Last N bytes.
			// Try seek optimization for regular files.
			if f, ok := reader.(*os.File); ok {
				if info, err := f.Stat(); err == nil && info.Mode().IsRegular() {
					size := info.Size()
					offset := max(size-int64(byteCount), 0)
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
			n := min(byteCount, maxTailBytes)
			if n == 0 {
				return nil
			}
			buf := make([]byte, n)
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
					space := n - head
					toCopy := min(nr-pos, space)
					copy(buf[head:head+toCopy], tmp[pos:pos+toCopy])
					head = (head + toCopy) % n
					pos += toCopy
					if filled < n {
						filled = min(filled+toCopy, n)
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
			if filled < n {
				// Buffer not yet full; data starts at 0.
				r.stdout.Write(buf[:filled])
			} else {
				// Full ring: oldest data starts at head.
				r.stdout.Write(buf[head:])
				if head > 0 {
					r.stdout.Write(buf[:head])
				}
			}
			return nil
		}

		// Line mode.
		lineSep := byte('\n')
		if zeroDelimFlag {
			lineSep = 0
		}

		if lineOffset >= 0 {
			// +N: from line N to end.
			toSkip := max(lineOffset-1, 0)
			scanner := newTailScanner(reader, lineSep)
			skipped := 0
			for scanner.Scan() {
				if skipped < toSkip {
					skipped++
					continue
				}
				r.stdout.Write(scanner.Bytes())
				r.stdout.Write([]byte{lineSep})
			}
			return scanner.Err()
		}

		// Last N lines: ring buffer of strings.
		n := max(lineCount, 0)
		n = min(n, maxTailLines)
		if n == 0 {
			// Drain input, output nothing.
			_, err := io.Copy(io.Discard, reader)
			return err
		}
		ring := make([]string, n)
		head := 0
		count := 0
		scanner := newTailScanner(reader, lineSep)
		for scanner.Scan() {
			ring[head] = scanner.Text()
			head = (head + 1) % n
			count++
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		// Determine start position and how many lines to print.
		size := min(count, n)
		start := head - size
		if start < 0 {
			start += n
		}
		sep := "\n"
		if zeroDelimFlag {
			sep = "\x00"
		}
		for i := range size {
			idx := (start + i) % n
			fmt.Fprintf(r.stdout, "%s%s", ring[idx], sep)
		}
		return nil
	}

	if len(files) == 0 {
		if err := tailOutput(r.stdin, "", false); err != nil {
			fmt.Fprintf(r.stderr, "tail: %v\n", err)
			r.exitCode = 1
			return nil
		}
		r.exitCode = 0
		return nil
	}

	showHeader := (len(files) > 1 && !quietFlag) || verboseFlag
	if quietFlag {
		showHeader = false
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
			fmt.Fprintf(r.stderr, "tail: cannot open %q for reading: No such file or directory\n", f)
			hasError = true
			continue
		}
		if err := tailOutput(file, f, showHeader); err != nil {
			fmt.Fprintf(r.stderr, "tail: %s: %v\n", f, err)
			hasError = true
		}
		file.Close()
	}
	if hasError {
		r.exitCode = 1
	} else {
		r.exitCode = 0
	}
	return nil
}

// parseTailN parses a -n argument. Sets lineOffset if the value starts with '+',
// otherwise sets lineCount. Negative values are rejected.
func parseTailN(raw string, lineCount, lineOffset *int) error {
	if strings.HasPrefix(raw, "+") {
		n, err := strconv.Atoi(raw[1:])
		if err != nil {
			return err
		}
		if n < 0 {
			return fmt.Errorf("invalid negative offset")
		}
		*lineOffset = n
		*lineCount = -1
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return err
	}
	if n < 0 {
		return fmt.Errorf("invalid negative line count")
	}
	*lineCount = n
	*lineOffset = -1
	return nil
}

// parseTailC parses a -c argument. Sets byteOffset if the value starts with '+',
// otherwise sets byteCount. Negative values are rejected.
func parseTailC(raw string, byteCount, byteOffset *int) error {
	if strings.HasPrefix(raw, "+") {
		n, err := strconv.Atoi(raw[1:])
		if err != nil {
			return err
		}
		if n < 0 {
			return fmt.Errorf("invalid negative offset")
		}
		*byteOffset = n
		*byteCount = -1
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return err
	}
	if n < 0 {
		return fmt.Errorf("invalid negative byte count")
	}
	*byteCount = n
	*byteOffset = -1
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
