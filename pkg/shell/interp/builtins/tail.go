// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

// Package builtins implements POSIX shell builtin commands.
//
// tail — output the last part of files
//
// Usage: tail [OPTION]... [FILE]...
//
// Print the last N lines (default 10) of each FILE to standard output.
// With more than one FILE, precede each chunk with a header giving the file name.
// With no FILE, or when FILE is -, read standard input.
//
// Supported flags:
//
//	-n N, --lines=N      Output the last N lines instead of the last 10.
//	                     If N begins with a '+', output starting with line N.
//	-c N, --bytes=N      Output the last N bytes.
//	                     If N begins with a '+', output starting with byte N.
//	-q, --quiet,
//	    --silent         Never print headers giving file names.
//	-v, --verbose        Always print headers giving file names.
//	-h, --help           Print this help message and exit.
//
// Explicitly unsupported flags (rejected with exit code 1):
//
//	-f, --follow         Follow file growth — would block indefinitely; not permitted.
//	-F                   Like --follow=name with retry — same reason.
//	--retry              Meaningful only with --follow.
//	--pid=PID            Meaningful only with --follow.
//	--sleep-interval=N   Meaningful only with --follow.
//
// All other unknown flags are rejected automatically by pflag.
//
// Resource limits (prevents DoS against infinite/huge sources):
//
//	tailMaxLines     = 1_000_000   lines stored in the ring buffer
//	tailMaxLineBytes = 256 KiB     maximum bytes per line (longer lines are capped)
//	tailMaxBytes     = 256 MiB     maximum bytes stored in the byte-mode ring buffer
//
// Exit codes:
//
//	0  All files processed successfully.
//	1  One or more files could not be opened or read (processing continues for remaining files).

package builtins

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/spf13/pflag"
)

// Resource limits — prevent DoS via large N or infinite sources.
const (
	tailMaxLines     = 1_000_000
	tailMaxLineBytes = 256 * 1024
	tailMaxBytes     = 256 * 1024 * 1024
)

// tailCountMode distinguishes last-N from from-offset-N semantics.
type tailCountMode int

const (
	tailCountLast tailCountMode = iota // output last N lines/bytes (default)
	tailCountFrom                      // output starting from line/byte N (1-based)
)

// tailCount holds a parsed count value and its mode.
type tailCount struct {
	n    int
	mode tailCountMode
}

// parseTailCount parses a count string for tail -n or -c.
// Accepted forms:
//   - "N"   → last N (tailCountLast)
//   - "+N"  → from line/byte N to EOF (tailCountFrom); +0 treated as +1
//   - "-N"  → same as "N" (GNU compat: - prefix means last)
func parseTailCount(s, kind string) (tailCount, error) {
	if s == "" {
		return tailCount{}, fmt.Errorf("invalid number of %s: %q", kind, s)
	}

	mode := tailCountLast
	digits := s
	if s[0] == '+' {
		mode = tailCountFrom
		digits = s[1:]
	} else if s[0] == '-' {
		digits = s[1:]
	}

	if digits == "" {
		return tailCount{}, fmt.Errorf("invalid number of %s: %q", kind, s)
	}

	n64, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		// Range overflow means the number is astronomically large; clamp to MaxInt.
		if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange {
			return tailCount{n: math.MaxInt, mode: mode}, nil
		}
		return tailCount{}, fmt.Errorf("invalid number of %s: %q", kind, s)
	}

	n := int(n64)
	if n64 > math.MaxInt {
		n = math.MaxInt
	}

	// +0 is treated as +1: output from the very beginning.
	if mode == tailCountFrom && n == 0 {
		n = 1
	}

	return tailCount{n: n, mode: mode}, nil
}

// builtinTail implements the tail command.
func builtinTail(ctx context.Context, callCtx *CallContext, args []string) Result {
	fs := pflag.NewFlagSet("tail", pflag.ContinueOnError)
	fs.SetOutput(io.Discard) // suppress pflag's default error output; we format it ourselves

	linesStr := fs.StringP("lines", "n", "", "output last N lines (default 10), or use +N to start at line N")
	bytesStr := fs.StringP("bytes", "c", "", "output last N bytes, or use +N to start at byte N")
	quietVal := false
	fs.BoolVarP(&quietVal, "quiet", "q", false, "never print headers giving file names")
	fs.BoolVar(&quietVal, "silent", false, "never print headers giving file names (alias for --quiet)")
	verbose := fs.BoolP("verbose", "v", false, "always print headers giving file names")
	help := fs.BoolP("help", "h", false, "print usage and exit")

	if err := fs.Parse(args); err != nil {
		callCtx.Errf("tail: %v\n", err)
		return Result{Code: 1}
	}

	if *help {
		callCtx.Out("Usage: tail [OPTION]... [FILE]...\n\n")
		callCtx.Out("Print the last 10 lines of each FILE to standard output.\n")
		callCtx.Out("With no FILE, or when FILE is -, read standard input.\n\n")
		fs.SetOutput(callCtx.Stdout)
		fs.PrintDefaults()
		return Result{}
	}

	// Determine output mode: bytes or lines, and count.
	byteMode := false
	count := tailCount{n: 10, mode: tailCountLast} // POSIX default: last 10 lines

	// Use fs.Changed (not != "") so that an explicitly-passed empty string like
	// -n "" is rejected by parseTailCount rather than silently ignored.
	if fs.Changed("bytes") {
		c, err := parseTailCount(*bytesStr, "bytes")
		if err != nil {
			callCtx.Errf("tail: %v\n", err)
			return Result{Code: 1}
		}
		count = c
		byteMode = true
	}
	// -n overrides -c when both are given (POSIX leaves this unspecified; we match GNU).
	if fs.Changed("lines") {
		c, err := parseTailCount(*linesStr, "lines")
		if err != nil {
			callCtx.Errf("tail: %v\n", err)
			return Result{Code: 1}
		}
		count = c
		byteMode = false
	}

	files := fs.Args()
	if len(files) == 0 {
		files = []string{"-"}
	}

	// Print headers when: multiple files and -q not set, or single file with -v.
	printHeaders := (len(files) > 1 && !quietVal) || *verbose

	exitCode := uint8(0)
	for i, name := range files {
		if printHeaders {
			if i > 0 {
				callCtx.Out("\n")
			}
			callCtx.Outf("==> %s <==\n", name)
		}

		var rc io.ReadCloser
		if name == "-" {
			if callCtx.Stdin == nil {
				continue
			}
			rc = io.NopCloser(callCtx.Stdin)
		} else {
			f, err := callCtx.OpenFile(ctx, name, os.O_RDONLY, 0)
			if err != nil {
				callCtx.Errf("tail: %s: %v\n", name, err)
				exitCode = 1
				continue
			}
			rc = f
		}

		var err error
		if byteMode {
			err = tailBytes(ctx, rc, callCtx.Stdout, count)
		} else {
			err = tailLines(ctx, rc, callCtx.Stdout, count)
		}
		rc.Close()

		if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
			callCtx.Errf("tail: %s: %v\n", name, err)
			exitCode = 1
		}
	}

	return Result{Code: exitCode}
}

// tailLines outputs lines from r according to count, writing to w.
func tailLines(ctx context.Context, r io.Reader, w io.Writer, count tailCount) error {
	if count.mode == tailCountFrom {
		return tailLinesFrom(ctx, r, w, count.n)
	}
	return tailLinesLast(ctx, r, w, count.n)
}

// tailBytes outputs bytes from r according to count, writing to w.
func tailBytes(ctx context.Context, r io.Reader, w io.Writer, count tailCount) error {
	if count.mode == tailCountFrom {
		return tailBytesFrom(ctx, r, w, count.n)
	}
	return tailBytesLast(ctx, r, w, count.n)
}

// tailLinesLast reads all of r and outputs the last n lines.
// Uses a ring buffer of capacity min(n, tailMaxLines).
// Lines longer than tailMaxLineBytes are capped.
func tailLinesLast(ctx context.Context, r io.Reader, w io.Writer, n int) error {
	if n == 0 {
		// Drain the reader to avoid broken-pipe errors in callers.
		return drainReader(ctx, r)
	}

	cap := n
	if cap > tailMaxLines {
		cap = tailMaxLines
	}

	ring := make([][]byte, cap)
	head := 0 // next write slot
	size := 0 // entries currently in ring

	var buf [4096]byte
	var line []byte
	prevCR := false // carry-over CR across chunk boundaries (for CRLF detection)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		nr, readErr := r.Read(buf[:])
		for i := 0; i < nr; i++ {
			b := buf[i]

			// Handle carry-over CR from the end of the previous chunk.
			// At this point `line` already ends with CR; decide whether to
			// complete it as CRLF (append LF and commit) or treat it as a
			// lone-CR line ending (commit without LF, then process b normally).
			if prevCR {
				prevCR = false
				if b == '\n' {
					line = appendLineByte(line, b)
					commitLine(&ring, &head, &size, cap, line)
					line = line[:0]
					continue
				}
				// Lone CR: commit the line that ends with CR, then process b.
				commitLine(&ring, &head, &size, cap, line)
				line = line[:0]
				// Fall through: b is the first byte of the next line.
			}

			if b == '\r' {
				line = appendLineByte(line, b)
				// Check if next byte is LF (CRLF).
				if i+1 < nr {
					if buf[i+1] == '\n' {
						i++
						line = appendLineByte(line, buf[i])
					}
					commitLine(&ring, &head, &size, cap, line)
					line = line[:0]
				} else {
					// CR is the last byte of the chunk; defer the commit until
					// we know whether LF follows in the next chunk.
					prevCR = true
				}
				continue
			}

			line = appendLineByte(line, b)
			if b == '\n' {
				commitLine(&ring, &head, &size, cap, line)
				line = line[:0]
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	// Commit any trailing line that has no terminator.
	if len(line) > 0 {
		commitLine(&ring, &head, &size, cap, line)
	}

	// Output ring buffer contents in order.
	start := (head - size + cap) % cap
	for i := 0; i < size; i++ {
		idx := (start + i) % cap
		if _, err := w.Write(ring[idx]); err != nil {
			return err
		}
	}
	return nil
}

// appendLineByte appends b to line, capping at tailMaxLineBytes.
// Bytes beyond the cap are silently dropped; the line still terminates normally.
func appendLineByte(line []byte, b byte) []byte {
	if len(line) < tailMaxLineBytes {
		return append(line, b)
	}
	return line
}

// commitLine stores a copy of line into the ring buffer, advancing head.
func commitLine(ring *[][]byte, head, size *int, cap int, line []byte) {
	cp := make([]byte, len(line))
	copy(cp, line)
	(*ring)[*head] = cp
	*head = (*head + 1) % cap
	if *size < cap {
		*size++
	}
}

// tailLinesFrom skips the first fromLine-1 lines, then copies the rest of r to w.
func tailLinesFrom(ctx context.Context, r io.Reader, w io.Writer, fromLine int) error {
	if fromLine <= 1 {
		return contextCopy(ctx, w, r)
	}

	skip := fromLine - 1
	var buf [4096]byte
	prevCR := false

	for skip > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		nr, readErr := r.Read(buf[:])
		i := 0

		if prevCR && nr > 0 {
			prevCR = false
			if buf[0] == '\n' {
				i = 1 // consume the LF part of the CRLF
			}
			// Either way the CR already counted as a line ending.
		}

		for ; i < nr && skip > 0; i++ {
			b := buf[i]
			if b == '\n' {
				skip--
			} else if b == '\r' {
				skip--
				if i+1 < nr {
					if buf[i+1] == '\n' {
						i++ // consume LF of CRLF
					}
				} else {
					prevCR = true
				}
			}
		}

		if skip == 0 {
			// Write remaining bytes of this chunk, then copy the rest.
			if i < nr {
				if _, err := w.Write(buf[i:nr]); err != nil {
					return err
				}
			}
			if readErr == io.EOF {
				return nil
			}
			return contextCopy(ctx, w, r)
		}

		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}

	return contextCopy(ctx, w, r)
}

// tailBytesLast reads all of r and outputs the last n bytes using a circular buffer.
func tailBytesLast(ctx context.Context, r io.Reader, w io.Writer, n int) error {
	if n == 0 {
		return drainReader(ctx, r)
	}

	bufSize := n
	if bufSize > tailMaxBytes {
		bufSize = tailMaxBytes
	}

	ring := make([]byte, bufSize)
	pos := 0 // total bytes written (mod bufSize = next write slot)

	var chunk [4096]byte
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		nr, readErr := r.Read(chunk[:])
		for i := 0; i < nr; i++ {
			ring[pos%bufSize] = chunk[i]
			pos++
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	// Output the ring buffer in order.
	if pos <= bufSize {
		_, err := w.Write(ring[:pos])
		return err
	}
	start := pos % bufSize
	if _, err := w.Write(ring[start:]); err != nil {
		return err
	}
	_, err := w.Write(ring[:start])
	return err
}

// tailBytesFrom skips the first fromByte-1 bytes, then copies the rest to w.
func tailBytesFrom(ctx context.Context, r io.Reader, w io.Writer, fromByte int) error {
	if fromByte <= 1 {
		return contextCopy(ctx, w, r)
	}

	skip := int64(fromByte - 1)
	var buf [4096]byte

	for skip > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		toRead := int64(len(buf))
		if toRead > skip {
			toRead = skip
		}

		nr, readErr := r.Read(buf[:toRead])
		skip -= int64(nr)

		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}

	return contextCopy(ctx, w, r)
}

// contextCopy copies from r to w, checking ctx.Err() before every read.
func contextCopy(ctx context.Context, w io.Writer, r io.Reader) error {
	var buf [4096]byte
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		nr, readErr := r.Read(buf[:])
		if nr > 0 {
			if _, writeErr := w.Write(buf[:nr]); writeErr != nil {
				return writeErr
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

// drainReader consumes r until EOF, respecting context cancellation.
func drainReader(ctx context.Context, r io.Reader) error {
	var buf [4096]byte
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, readErr := r.Read(buf[:])
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
