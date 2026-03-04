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
)

const (
	maxTailLines = 10_000           // max ring-buffer capacity (lines)
	maxLineBytes = 1 * 1024 * 1024  // 1 MB per-line scanner cap
	maxTailBytes = 10 * 1024 * 1024 // 10 MB ring buffer for byte mode
)

func (r *Runner) builtinTail(ctx context.Context, args []string) error {
	lineCount := 10  // default last N lines
	byteCount := -1  // -1 means "not set"
	lineOffset := -1 // +N: from line N to end (-1 = not set)
	byteOffset := -1 // +N: from byte N to end (-1 = not set)
	quiet := false
	verbose := false
	zeroDelim := false
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

		// Reject follow-mode flags immediately — they block forever.
		switch a {
		case "-f", "--follow", "-F", "--retry", "--pid",
			"--max-unchanged-stats":
			return fmt.Errorf("flag %q is not allowed for command \"tail\": follow/retry flags cause infinite blocking loops", a)
		}
		if strings.HasPrefix(a, "-s") || a == "--sleep-interval" || strings.HasPrefix(a, "--sleep-interval=") {
			return fmt.Errorf("flag %q is not allowed for command \"tail\": follow/retry flags cause infinite blocking loops", a)
		}

		switch {
		case a == "-n" || a == "--lines":
			i++
			if i >= len(args) {
				fmt.Fprintf(r.stderr, "tail: option requires an argument -- 'n'\n")
				r.exitCode = 1
				return nil
			}
			if err := parseTailN(args[i], &lineCount, &lineOffset); err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", args[i])
				r.exitCode = 1
				return nil
			}
			byteCount = -1
			byteOffset = -1

		case strings.HasPrefix(a, "-n"):
			raw := a[2:]
			if err := parseTailN(raw, &lineCount, &lineOffset); err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", raw)
				r.exitCode = 1
				return nil
			}
			byteCount = -1
			byteOffset = -1

		case strings.HasPrefix(a, "--lines="):
			raw := a[len("--lines="):]
			if err := parseTailN(raw, &lineCount, &lineOffset); err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", raw)
				r.exitCode = 1
				return nil
			}
			byteCount = -1
			byteOffset = -1

		case a == "-c" || a == "--bytes":
			i++
			if i >= len(args) {
				fmt.Fprintf(r.stderr, "tail: option requires an argument -- 'c'\n")
				r.exitCode = 1
				return nil
			}
			if err := parseTailC(args[i], &byteCount, &byteOffset); err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", args[i])
				r.exitCode = 1
				return nil
			}
			lineCount = -1
			lineOffset = -1

		case strings.HasPrefix(a, "-c"):
			raw := a[2:]
			if err := parseTailC(raw, &byteCount, &byteOffset); err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", raw)
				r.exitCode = 1
				return nil
			}
			lineCount = -1
			lineOffset = -1

		case strings.HasPrefix(a, "--bytes="):
			raw := a[len("--bytes="):]
			if err := parseTailC(raw, &byteCount, &byteOffset); err != nil {
				fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", raw)
				r.exitCode = 1
				return nil
			}
			lineCount = -1
			lineOffset = -1

		case a == "-q" || a == "--quiet" || a == "--silent":
			quiet = true

		case a == "-v" || a == "--verbose":
			verbose = true

		case a == "-z" || a == "--zero-terminated":
			zeroDelim = true

		default:
			return fmt.Errorf("flag %q is not allowed for command \"tail\"", a)
		}
	}

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
		if zeroDelim {
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
		if zeroDelim {
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

	showHeader := (len(files) > 1 && !quiet) || verbose
	if quiet {
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
