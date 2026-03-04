// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

// builtinTail implements the POSIX tail command as a safe shell builtin.
//
// Usage: tail [OPTION]... [FILE]...
//
// Print the last 10 lines of each FILE to standard output.
// With more than one FILE, precede each with a header giving the file name.
// With no FILE, or when FILE is -, read standard input.
//
// Supported flags:
//
//	-n, --lines=[+]NUM   output the last NUM lines; with leading '+', output
//	                     starting with line NUM (1-indexed)
//	-c, --bytes=[+]NUM   output the last NUM bytes; with leading '+', output
//	                     starting with byte NUM (1-indexed)
//	-q, --quiet, --silent never output headers giving file names
//	-v, --verbose         always output headers giving file names
//	-h, --help            display usage information and exit
//
// Rejected flags (not implemented):
//
//	-f, --follow          rejected — infinite blocking / DoS vector
//	-F                    rejected — same as --follow=name --retry
//	--retry               rejected — infinite retry loop
//	--pid=PID             rejected — only meaningful with -f
//	-s, --sleep-interval  rejected — only meaningful with -f
//	--max-unchanged-stats rejected — only meaningful with -f
//	-z, --zero-terminated rejected — not implemented
//
// All unrecognized flags are automatically rejected by the flag parser.

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	pflag "github.com/spf13/pflag"
)

// Resource limits for tail to prevent memory exhaustion.
const (
	tailMaxLines     = 1_000_000       // max ring buffer size for last-N-lines mode
	tailMaxLineBytes = 256 * 1024      // 256 KiB max single line length
	tailMaxBytes     = 256 * 1024 * 1024 // 256 MiB max byte-mode buffer
)

// tailMode distinguishes between line-counting and byte-counting modes.
type tailMode int

const (
	tailModeLines tailMode = iota
	tailModeBytes
)

func (r *Runner) builtinTail(ctx context.Context, args []string) error {
	fs := pflag.NewFlagSet("tail", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)

	linesStr := fs.StringP("lines", "n", "", "number of lines")
	bytesStr := fs.StringP("bytes", "c", "", "number of bytes")
	quiet := fs.BoolP("quiet", "q", false, "never output headers")
	fs.Bool("silent", false, "never output headers")
	verbose := fs.BoolP("verbose", "v", false, "always output headers")
	help := fs.BoolP("help", "h", false, "display help")

	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintf(r.stderr, "tail: %s\n", err)
		r.exitCode = 1
		return nil
	}

	if *help {
		_, _ = fmt.Fprintln(r.stdout, "Usage: tail [OPTION]... [FILE]...")
		_, _ = fmt.Fprintln(r.stdout, "Print the last 10 lines of each FILE to standard output.")
		_, _ = fmt.Fprintln(r.stdout)
		fs.SetOutput(r.stdout)
		fs.PrintDefaults()
		r.exitCode = 0
		return nil
	}

	// --silent is an alias for --quiet
	if fs.Changed("silent") {
		*quiet = true
	}

	// Parse the mode and count. When both -c and -n are given, -n wins
	// (it is checked last). POSIX leaves this unspecified.
	mode := tailModeLines
	count := 10
	isFromOffset := false // true when +N syntax is used

	if *bytesStr != "" {
		mode = tailModeBytes
		var err error
		count, isFromOffset, err = parseTailCount(*bytesStr)
		if err != nil {
			_, _ = fmt.Fprintf(r.stderr, "tail: invalid number of bytes: %q\n", *bytesStr)
			r.exitCode = 1
			return nil
		}
	}
	if *linesStr != "" {
		mode = tailModeLines
		var err error
		count, isFromOffset, err = parseTailCount(*linesStr)
		if err != nil {
			_, _ = fmt.Fprintf(r.stderr, "tail: invalid number of lines: %q\n", *linesStr)
			r.exitCode = 1
			return nil
		}
	}

	files := fs.Args()
	if len(files) == 0 {
		files = []string{"-"}
	}

	// Determine header policy.
	showHeaders := len(files) > 1
	if *quiet {
		showHeaders = false
	}
	if *verbose {
		showHeaders = true
	}

	hasError := false
	for i, name := range files {
		if ctx.Err() != nil {
			break
		}

		if err := r.tailOneFile(ctx, name, mode, count, isFromOffset, showHeaders, i > 0); err != nil {
			_, _ = fmt.Fprintf(r.stderr, "tail: %s: %s\n", name, err)
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

// tailOneFile processes a single file for tail. It returns an error only for
// the file-level issues (missing file, directory, etc.) — not for write errors
// on stdout which are returned by the underlying tail functions.
func (r *Runner) tailOneFile(ctx context.Context, name string, mode tailMode, count int, isFromOffset, showHeader, printBlankBefore bool) error {
	if showHeader {
		if printBlankBefore {
			_, _ = fmt.Fprintln(r.stdout)
		}
		_, _ = fmt.Fprintf(r.stdout, "==> %s <==\n", name)
	}

	var reader io.Reader
	if name == "-" {
		reader = r.stdin
	} else {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(r.dir, path)
		}

		if runtime.GOOS == "windows" && isWindowsReservedName(filepath.Base(path)) {
			return fmt.Errorf("reserved file name")
		}

		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("Is a directory")
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		reader = f
	}

	if mode == tailModeBytes {
		return tailBytes(ctx, reader, r.stdout, count, isFromOffset)
	}
	return tailLines(ctx, reader, r.stdout, count, isFromOffset)
}

// parseTailCount parses a tail count string which may have a leading '+'.
// Returns the count, whether it's a from-offset (+N), and any error.
// Negative numbers are rejected. Overflow is clamped to math.MaxInt.
func parseTailCount(s string) (count int, isFromOffset bool, err error) {
	if s == "" {
		return 0, false, fmt.Errorf("empty count")
	}

	if strings.HasPrefix(s, "+") {
		isFromOffset = true
		s = s[1:]
	} else if strings.HasPrefix(s, "-") {
		return 0, false, fmt.Errorf("negative count")
	}

	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		// Check for overflow — clamp to MaxInt.
		if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
			return math.MaxInt, isFromOffset, nil
		}
		return 0, false, err
	}

	if n > uint64(math.MaxInt) {
		return math.MaxInt, isFromOffset, nil
	}
	return int(n), isFromOffset, nil
}

// tailLines outputs lines from reader according to count and isFromOffset.
func tailLines(ctx context.Context, reader io.Reader, w io.Writer, count int, isFromOffset bool) error {
	if isFromOffset {
		return tailLinesFromOffset(ctx, reader, w, count)
	}
	return tailLinesLast(ctx, reader, w, count)
}

// tailLinesFromOffset outputs lines starting from line number `offset` (1-indexed).
// +0 is treated the same as +1 (output everything).
func tailLinesFromOffset(ctx context.Context, reader io.Reader, w io.Writer, offset int) error {
	if offset <= 1 {
		offset = 1
	}
	// Skip offset-1 lines, then copy the rest.
	linesToSkip := offset - 1
	lineNum := 0
	buf := make([]byte, 32*1024)
	skipping := true
	var carry []byte

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		n, readErr := reader.Read(buf)
		if n > 0 {
			data := buf[:n]
			if carry != nil {
				data = append(carry, data...)
				carry = nil
			}

			if skipping {
				for len(data) > 0 {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					idx := findLineEnd(data)
					if idx < 0 {
						// No line ending found; carry forward.
						// Clamp carried data to prevent unbounded growth.
						if len(data) > tailMaxLineBytes {
							data = data[len(data)-tailMaxLineBytes:]
						}
						carry = make([]byte, len(data))
						copy(carry, data)
						break
					}
					lineNum++
					if lineNum > linesToSkip {
						// We've skipped enough; output this line and everything after.
						skipping = false
						if _, err := w.Write(data[:idx+1]); err != nil {
							return err
						}
						remaining := data[idx+1:]
						if len(remaining) > 0 {
							if _, err := w.Write(remaining); err != nil {
								return err
							}
						}
						break
					}
					data = data[idx+1:]
				}
			} else {
				if _, err := w.Write(data); err != nil {
					return err
				}
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				// If we have leftover carry and we're still skipping, check
				// if we've skipped enough lines (the last line has no terminator).
				if skipping && carry != nil {
					lineNum++
					if lineNum > linesToSkip {
						if _, err := w.Write(carry); err != nil {
							return err
						}
					}
				}
				return nil
			}
			return readErr
		}
	}
}

// tailLinesLast outputs the last `count` lines from reader using a ring buffer.
func tailLinesLast(ctx context.Context, reader io.Reader, w io.Writer, count int) error {
	if count == 0 {
		// Consume input (respect context) but output nothing.
		return discardReader(ctx, reader)
	}

	// Clamp to prevent OOM.
	ringSize := count
	if ringSize > tailMaxLines {
		ringSize = tailMaxLines
	}

	// Ring buffer of lines. Each entry stores the full line including its terminator.
	ring := make([][]byte, ringSize)
	ringIdx := 0

	buf := make([]byte, 32*1024)
	var carry []byte

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		n, readErr := reader.Read(buf)
		if n > 0 {
			data := buf[:n]
			if carry != nil {
				data = append(carry, data...)
				carry = nil
			}

			for len(data) > 0 {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				idx := findLineEnd(data)
				if idx < 0 {
					// No line ending; carry forward (clamped).
					if len(data) > tailMaxLineBytes {
						data = data[len(data)-tailMaxLineBytes:]
					}
					carry = make([]byte, len(data))
					copy(carry, data)
					break
				}

				line := make([]byte, idx+1)
				copy(line, data[:idx+1])
				ring[ringIdx%ringSize] = line
				ringIdx++
				data = data[idx+1:]
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	// If there's carry data (last line with no terminator), add it to the ring.
	if carry != nil {
		line := make([]byte, len(carry))
		copy(line, carry)
		ring[ringIdx%ringSize] = line
		ringIdx++
	}

	// Output the ring buffer contents.
	numToOutput := ringIdx
	if numToOutput > ringSize {
		numToOutput = ringSize
	}
	if numToOutput > count {
		numToOutput = count
	}

	startIdx := ringIdx - numToOutput
	for i := 0; i < numToOutput; i++ {
		line := ring[(startIdx+i)%ringSize]
		if _, err := w.Write(line); err != nil {
			return err
		}
	}

	return nil
}

// tailBytes outputs bytes from reader according to count and isFromOffset.
func tailBytes(ctx context.Context, reader io.Reader, w io.Writer, count int, isFromOffset bool) error {
	if isFromOffset {
		return tailBytesFromOffset(ctx, reader, w, count)
	}
	return tailBytesLast(ctx, reader, w, count)
}

// tailBytesFromOffset outputs bytes starting from byte number `offset` (1-indexed).
// +0 is treated the same as +1 (output everything).
func tailBytesFromOffset(ctx context.Context, reader io.Reader, w io.Writer, offset int) error {
	if offset <= 1 {
		offset = 1
	}

	// Skip offset-1 bytes, then copy the rest.
	toSkip := offset - 1
	buf := make([]byte, 32*1024)

	for toSkip > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		readSize := len(buf)
		if readSize > toSkip {
			readSize = toSkip
		}
		n, err := reader.Read(buf[:readSize])
		toSkip -= n
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}

	// Copy remaining.
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := reader.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// tailBytesLast outputs the last `count` bytes from reader using a circular buffer.
func tailBytesLast(ctx context.Context, reader io.Reader, w io.Writer, count int) error {
	if count == 0 {
		return discardReader(ctx, reader)
	}

	// Clamp buffer size.
	bufSize := count
	if bufSize > tailMaxBytes {
		bufSize = tailMaxBytes
	}

	ring := make([]byte, bufSize)
	ringPos := 0
	totalRead := 0

	readBuf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		n, err := reader.Read(readBuf)
		for i := 0; i < n; i++ {
			ring[ringPos%bufSize] = readBuf[i]
			ringPos++
		}
		totalRead += n

		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	// Output the buffered bytes.
	bytesAvailable := totalRead
	if bytesAvailable > bufSize {
		bytesAvailable = bufSize
	}
	if bytesAvailable > count {
		bytesAvailable = count
	}

	startPos := ringPos - bytesAvailable
	if bytesAvailable == bufSize && startPos%bufSize == 0 {
		// Ring is full and we start at the write position.
		if _, err := w.Write(ring); err != nil {
			return err
		}
	} else if startPos%bufSize+bytesAvailable <= bufSize {
		// Data is contiguous.
		if _, err := w.Write(ring[startPos%bufSize : startPos%bufSize+bytesAvailable]); err != nil {
			return err
		}
	} else {
		// Data wraps around.
		start := startPos % bufSize
		firstChunk := bufSize - start
		if _, err := w.Write(ring[start:]); err != nil {
			return err
		}
		if _, err := w.Write(ring[:bytesAvailable-firstChunk]); err != nil {
			return err
		}
	}

	return nil
}

// findLineEnd finds the index of the first line ending (LF, CR, or CRLF) in data.
// Returns -1 if no line ending is found. For CRLF, returns the index of the LF.
func findLineEnd(data []byte) int {
	for i, b := range data {
		if b == '\n' {
			return i
		}
		if b == '\r' {
			// Check if this is CRLF.
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 1 // return index of LF in CRLF
			}
			// Standalone CR is a line ending.
			return i
		}
	}
	return -1
}

// discardReader reads and discards all data from reader, respecting context.
func discardReader(ctx context.Context, reader io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// isWindowsReservedName checks if a filename is a Windows reserved device name.
func isWindowsReservedName(name string) bool {
	// Strip extension if present.
	if dot := strings.IndexByte(name, '.'); dot >= 0 {
		name = name[:dot]
	}
	upper := strings.ToUpper(name)
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	// COM1-COM9, LPT1-LPT9
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		d := upper[3]
		if d >= '1' && d <= '9' {
			return true
		}
	}
	return false
}
