// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// builtinTail implements a safe subset of POSIX tail.
//
// Supported flags:
//
//	-n N / --lines=N   output the last N lines (default 10); "+N" means from line N
//	-c N / --bytes=N   output the last N bytes; "+N" means from byte N
//	-q / --quiet / --silent  never print file headers
//	-v / --verbose     always print file headers
//	-h / --help        print usage to stdout, exit 0
//
// Unsupported flags are rejected by pflag with exit code 1.
// Notably: -f/--follow, -F, --retry, --pid, --sleep-interval, -r are all
// unsupported and will be rejected automatically.
//
// Resource limits:
//
//	tailMaxLines     = 1_000_000   ring-buffer cap for last-N lines mode
//	tailMaxLineBytes = 256 KiB     max bytes per line (longer lines are truncated)
//	tailMaxBytes     = 256 MiB     ring-buffer cap for last-N bytes mode
package interp

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

const (
	tailMaxLines     = 1_000_000
	tailMaxLineBytes = 256 * 1024
	tailMaxBytes     = 256 * 1024 * 1024
)

type tailMode int

const (
	tailModeLines tailMode = iota
	tailModeBytes
)

func (r *Runner) builtinTail(ctx context.Context, args []string) error {
	fs := pflag.NewFlagSet("tail", pflag.ContinueOnError)
	fs.SetOutput(io.Discard) // suppress pflag's own error output; we format errors ourselves

	lines := fs.StringP("lines", "n", "10", "output the last K lines (or use +K to start from line K)")
	bytesFlag := fs.StringP("bytes", "c", "", "output the last K bytes (or use +K to start from byte K)")
	quiet := fs.BoolP("quiet", "q", false, "never print file headers")
	_ = fs.Bool("silent", false, "never print file headers") // alias for --quiet
	verbose := fs.BoolP("verbose", "v", false, "always print file headers")
	help := fs.BoolP("help", "h", false, "show this help and exit")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(r.stderr, "tail: %v\n", err)
		r.exitCode = 1
		return nil
	}

	// --silent is an alias for --quiet
	if fs.Changed("silent") {
		*quiet = true
	}

	if *help {
		fmt.Fprintf(r.stdout, "Usage: tail [OPTION]... [FILE]...\n")
		fmt.Fprintf(r.stdout, "Output the last lines of each FILE to stdout.\n\n")
		fs.SetOutput(r.stdout)
		fs.PrintDefaults()
		r.exitCode = 0
		return nil
	}

	files := fs.Args()

	// Parse -n / --lines value
	mode := tailModeLines
	lineCount := 10
	fromLine := 0
	isFromLine := false

	if fs.Changed("lines") {
		var err error
		isFromLine, lineCount, fromLine, err = parseTailCount(*lines)
		if err != nil {
			fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", *lines)
			r.exitCode = 1
			return nil
		}
	}

	// Parse -c / --bytes value (overrides -n)
	byteCount := 0
	fromByte := 0
	isFromByte := false

	if fs.Changed("bytes") {
		mode = tailModeBytes
		var err error
		isFromByte, byteCount, fromByte, err = parseTailCount(*bytesFlag)
		if err != nil {
			fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", *bytesFlag)
			r.exitCode = 1
			return nil
		}
	}

	// showHeader logic: show headers for multiple files unless -q; always if -v.
	showHeaderForMultiple := !*quiet && len(files) > 1
	showHeaderAlways := *verbose && !*quiet

	tailOutput := func(reader io.Reader, name string, showHeader bool) error {
		if showHeader {
			fmt.Fprintf(r.stdout, "==> %s <==\n", name)
		}
		switch mode {
		case tailModeBytes:
			if isFromByte {
				return tailFromByte(ctx, r.stdout, reader, fromByte)
			}
			n := min(byteCount, tailMaxBytes)
			return tailLastBytes(ctx, r.stdout, reader, n)
		default: // tailModeLines
			if isFromLine {
				return tailFromLine(ctx, r.stdout, reader, fromLine)
			}
			n := min(lineCount, tailMaxLines)
			return tailLastLines(ctx, r.stdout, reader, n)
		}
	}

	if len(files) == 0 {
		if err := tailOutput(r.stdin, "", false); err != nil {
			r.exitCode = 1
			return nil
		}
		r.exitCode = 0
		return nil
	}

	hasError := false
	for idx, f := range files {
		showHeader := showHeaderForMultiple || showHeaderAlways
		if idx > 0 && showHeader {
			fmt.Fprintln(r.stdout)
		}
		path := f
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.dir, path)
		}
		// Reject Windows reserved device names (CON, NUL, COM1, etc.) to prevent hangs.
		if runtime.GOOS == "windows" && isWindowsReservedName(f) {
			fmt.Fprintf(r.stderr, "tail: cannot open %q for reading: No such file or directory\n", f)
			hasError = true
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			// Use the OS error cause (e.g. "is a directory", "permission denied")
			// rather than a hardcoded string, so error messages are actionable.
			cause := err
			var pathErr *os.PathError
			if errors.As(err, &pathErr) {
				cause = pathErr.Err
			}
			fmt.Fprintf(r.stderr, "tail: cannot open %q for reading: %s\n", f, cause)
			hasError = true
			continue
		}
		if err := tailOutput(file, f, showHeader); err != nil {
			fmt.Fprintf(r.stderr, "tail: error reading %q: %s\n", f, err)
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

// parseTailCount parses a tail count value, optionally with leading '+'.
// Returns (isOffset, lastN, fromN, err).
// isOffset=true means "+N" syntax was used (from-offset mode).
// Overflow-safe: very large numbers are returned as math.MaxInt to be clamped by callers.
func parseTailCount(s string) (isOffset bool, lastN int, fromN int, err error) {
	if strings.HasPrefix(s, "+") {
		n, perr := parseTailInt(s[1:])
		if perr != nil {
			return false, 0, 0, fmt.Errorf("invalid value")
		}
		return true, 0, n, nil
	}
	n, perr := parseTailInt(s)
	if perr != nil {
		return false, 0, 0, fmt.Errorf("invalid value")
	}
	return false, n, 0, nil
}

// parseTailInt parses a non-negative integer string with overflow protection.
// Returns an error on empty string (matching GNU tail behavior).
// Returns math.MaxInt on overflow (will be clamped to limits later).
func parseTailInt(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("invalid value")
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		// Check if it's just overflow; if so, return max int to be clamped.
		numErr, ok := err.(*strconv.NumError)
		if ok && numErr.Err == strconv.ErrRange {
			return math.MaxInt, nil
		}
		return 0, err
	}
	if n > math.MaxInt {
		return math.MaxInt, nil
	}
	return int(n), nil
}

// tailLastLines reads all input into a ring buffer and writes the last n lines.
// Handles CRLF, bare CR, and LF line endings (including mixed within the same file).
// Lines longer than tailMaxLineBytes are silently truncated; this is a deliberate
// resource limit to prevent memory exhaustion from pathological inputs.
func tailLastLines(ctx context.Context, w io.Writer, r io.Reader, n int) error {
	if n == 0 {
		return drainWithContext(ctx, r)
	}

	scanner := bufio.NewScanner(r)
	// +1 so the scanner can hold a full tailMaxLineBytes-byte line plus its terminator.
	scanner.Buffer(make([]byte, tailMaxLineBytes+1), tailMaxLineBytes+1)
	scanner.Split(scanLinesNoCR)

	ring := make([]string, n)
	count := 0

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		ring[count%n] = scanner.Text()
		count++
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Flush in order.
	start := 0
	size := count
	if size > n {
		start = count % n
		size = n
	}
	for i := 0; i < size; i++ {
		fmt.Fprintln(w, ring[(start+i)%n])
	}
	return nil
}

// tailFromLine skips the first (n-1) lines, then copies the rest.
func tailFromLine(ctx context.Context, w io.Writer, r io.Reader, n int) error {
	if n <= 0 {
		n = 1
	}
	scanner := bufio.NewScanner(r)
	// +1 so the scanner can hold a full tailMaxLineBytes-byte line plus its terminator.
	scanner.Buffer(make([]byte, tailMaxLineBytes+1), tailMaxLineBytes+1)
	scanner.Split(scanLinesNoCR)

	skipped := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		skipped++
		if skipped >= n {
			fmt.Fprintln(w, scanner.Text())
			break
		}
	}
	if scanner.Err() != nil {
		return scanner.Err()
	}
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Fprintln(w, scanner.Text())
	}
	return scanner.Err()
}

// tailLastBytes reads all input into a circular byte buffer and writes the last n bytes.
func tailLastBytes(ctx context.Context, w io.Writer, r io.Reader, n int) error {
	if n == 0 {
		return drainWithContext(ctx, r)
	}

	buf := make([]byte, n)
	total := 0
	chunk := make([]byte, 32*1024)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		nr, err := r.Read(chunk)
		if nr > 0 {
			for _, b := range chunk[:nr] {
				buf[total%n] = b
				total++
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	if total <= n {
		_, err := w.Write(buf[:total])
		return err
	}
	// Wrap: write from (total%n) to n, then 0 to (total%n).
	start := total % n
	if _, err := w.Write(buf[start:]); err != nil {
		return err
	}
	_, err := w.Write(buf[:start])
	return err
}

// tailFromByte skips the first (n-1) bytes, then copies the rest.
func tailFromByte(ctx context.Context, w io.Writer, r io.Reader, n int) error {
	if n <= 0 {
		n = 1
	}
	toSkip := int64(n - 1)
	if toSkip > 0 {
		skipped, err := io.CopyN(io.Discard, r, toSkip)
		if err == io.EOF || skipped < toSkip {
			return nil
		}
		if err != nil {
			return err
		}
	}

	chunk := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		nr, err := r.Read(chunk)
		if nr > 0 {
			if _, werr := w.Write(chunk[:nr]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// drainWithContext reads and discards r until EOF or context cancellation.
func drainWithContext(ctx context.Context, r io.Reader) error {
	chunk := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := r.Read(chunk)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// scanLinesNoCR is a bufio.SplitFunc that handles LF, CRLF, and bare CR line endings,
// stripping the line terminator from the returned token.
// It finds whichever line ending appears first in the buffer, so bare CR before a later
// LF is treated as a line separator (not as content), enabling correct handling of mixed
// line endings within the same file.
func scanLinesNoCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	crIdx := bytes.IndexByte(data, '\r')
	lfIdx := bytes.IndexByte(data, '\n')

	// CR comes before LF (or only CR present) — could be CRLF or bare CR
	if crIdx >= 0 && (lfIdx < 0 || crIdx < lfIdx) {
		if crIdx+1 >= len(data) {
			// CR is at end of buffer; need more data to distinguish CRLF from bare CR
			if !atEOF {
				return 0, nil, nil
			}
			// At EOF: bare CR is the final terminator
			return len(data), data[:crIdx], nil
		}
		if data[crIdx+1] == '\n' {
			// CRLF
			return crIdx + 2, data[:crIdx], nil
		}
		// Bare CR line ending
		return crIdx + 1, data[:crIdx], nil
	}

	// LF comes first (or only LF present)
	if lfIdx >= 0 {
		return lfIdx + 1, data[:lfIdx], nil
	}

	// No line ending in buffer
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// isWindowsReservedName reports whether the base name of path is a Windows
// reserved device name (CON, NUL, PRN, AUX, COM1-9, LPT1-9).
// Opening these names on Windows blocks indefinitely instead of returning an error.
// The check is only consulted when runtime.GOOS == "windows".
func isWindowsReservedName(path string) bool {
	base := strings.ToUpper(filepath.Base(path))
	// Strip any extension (e.g. NUL.txt → NUL)
	if i := strings.LastIndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	switch base {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}
	return false
}
