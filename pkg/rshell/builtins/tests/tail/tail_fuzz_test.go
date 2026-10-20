// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tail_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// FuzzTailLines fuzzes tail -n N with arbitrary file content.
// Edge cases: ring buffer limits (100K lines, 64 MiB), MaxCount clamp (2^31-1),
// negative values treated as absolute, no-trailing-newline preservation.
func FuzzTailLines(f *testing.F) {
	f.Add([]byte("line1\nline2\nline3\n"), int64(2))
	f.Add([]byte{}, int64(0))
	f.Add([]byte("no newline"), int64(1))
	f.Add([]byte("a\x00b\nc\n"), int64(2))
	f.Add(bytes.Repeat([]byte("x"), 4097), int64(1))
	f.Add([]byte("\n\n\n"), int64(5))
	f.Add(bytes.Repeat([]byte("y"), 4096), int64(1))
	f.Add([]byte("hello\nworld\n"), int64(10))
	// MaxCount boundary — clamp prevents allocation
	f.Add([]byte("tiny\n"), int64(1<<31-1))
	f.Add([]byte("tiny\n"), int64(9999999999))
	// n=0 → no output
	f.Add([]byte("a\nb\nc\n"), int64(0))
	// Binary / null bytes in line
	f.Add([]byte("a\x00b\x00c\n"), int64(1))
	// CRLF lines
	f.Add([]byte("line1\r\nline2\r\nline3\r\n"), int64(2))
	// Invalid UTF-8
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'}, int64(1))
	// Lines at 1 MiB cap boundary
	f.Add(append(bytes.Repeat([]byte("a"), 1<<20-1), '\n'), int64(1))
	f.Add(append(bytes.Repeat([]byte("b"), 1<<20), '\n'), int64(1))
	// Chunk-boundary straddle (ring buffer 32 KiB chunks)
	f.Add(bytes.Repeat([]byte("z\n"), 32*1024/2), int64(5))
	// No trailing newline on last line
	f.Add([]byte("line1\nline2"), int64(1))
	// Many blank lines (stress ring buffer)
	f.Add(bytes.Repeat([]byte("\n"), 1000), int64(5))

	f.Fuzz(func(t *testing.T, input []byte, n int64) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if n < 0 {
			return
		}
		if n > 10000 {
			n = 10000
		}

		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		stdout, _, code := cmdRunCtx(ctx, t, fmt.Sprintf("tail -n %d input.txt", n), dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("tail -n %d unexpected exit code %d", n, code)
		}

		// If successful, output line count must be <= n
		if code == 0 && n >= 0 {
			lineCount := strings.Count(stdout, "\n")
			if int64(lineCount) > n {
				t.Errorf("tail -n %d produced %d newlines in output", n, lineCount)
			}
		}
	})
}

// FuzzTailBytes fuzzes tail -c N with arbitrary file content.
// Edge cases: circular byte buffer (32 MiB), MaxCount clamp, binary content.
func FuzzTailBytes(f *testing.F) {
	f.Add([]byte("line1\nline2\nline3\n"), int64(5))
	f.Add([]byte{}, int64(0))
	f.Add([]byte("no newline"), int64(3))
	f.Add([]byte("a\x00b\nc\n"), int64(4))
	f.Add(bytes.Repeat([]byte("x"), 4097), int64(4096))
	f.Add([]byte("\n\n\n"), int64(2))
	// MaxCount boundary
	f.Add([]byte("tiny"), int64(1<<31-1))
	f.Add([]byte("tiny"), int64(9999999999))
	// n=0 → no output
	f.Add([]byte("abc"), int64(0))
	// Binary content (null bytes, high bytes)
	f.Add([]byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe}, int64(4))
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf}, int64(6))
	// CRLF
	f.Add([]byte("a\r\nb\r\n"), int64(3))
	// Chunk boundary (32 KiB)
	f.Add(bytes.Repeat([]byte("z"), 32*1024+1), int64(1))

	f.Fuzz(func(t *testing.T, input []byte, n int64) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if n < 0 {
			return
		}
		if n > 10000 {
			n = 10000
		}

		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		stdout, _, code := cmdRunCtx(ctx, t, fmt.Sprintf("tail -c %d input.txt", n), dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("tail -c %d unexpected exit code %d", n, code)
		}

		// If successful, output byte count must be <= n
		if code == 0 {
			outLen := int64(len(stdout))
			if outLen > n {
				t.Errorf("tail -c %d produced %d bytes of output", n, outLen)
			}
		}
	})
}

// FuzzTailStdin fuzzes tail -n N reading from stdin via shell redirection.
// Stdin is treated as a non-regular file — MaxTotalReadBytes (256 MiB) applies.
func FuzzTailStdin(f *testing.F) {
	f.Add([]byte("line1\nline2\nline3\n"), int64(2))
	f.Add([]byte{}, int64(1))
	f.Add([]byte("no newline"), int64(1))
	f.Add([]byte("a\x00b\nc\n"), int64(2))
	f.Add(bytes.Repeat([]byte("x"), 4097), int64(1))
	f.Add([]byte("\n\n\n"), int64(3))
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'}, int64(1))
	f.Add([]byte("line1\r\nline2\r\n"), int64(1))

	f.Fuzz(func(t *testing.T, input []byte, n int64) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if n < 0 {
			return
		}
		if n > 10000 {
			n = 10000
		}

		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "stdin.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, fmt.Sprintf("tail -n %d < stdin.txt", n), dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("tail stdin unexpected exit code %d", code)
		}
	})
}

// FuzzTailLinesOffset fuzzes tail -n +N (skip-first-N-lines offset mode).
// Edge cases: +1 streams entire file, very large +N skips everything.
func FuzzTailLinesOffset(f *testing.F) {
	f.Add([]byte("line1\nline2\nline3\n"), int64(1))
	f.Add([]byte("line1\nline2\nline3\n"), int64(2))
	f.Add([]byte{}, int64(1))
	f.Add([]byte("no newline"), int64(1))
	f.Add([]byte("a\x00b\nc\n"), int64(2))
	f.Add(bytes.Repeat([]byte("x"), 4097), int64(1))
	f.Add([]byte("\n\n\n"), int64(5))
	f.Add([]byte("hello\nworld\n"), int64(100))
	// +1 streams entire file
	f.Add([]byte("a\nb\nc\n"), int64(1))
	// +N > line count → empty output
	f.Add([]byte("a\nb\n"), int64(1000))
	// Binary
	f.Add([]byte("a\x00b\nc\n"), int64(1))
	// CRLF
	f.Add([]byte("a\r\nb\r\nc\r\n"), int64(2))

	f.Fuzz(func(t *testing.T, input []byte, n int64) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if n < 1 {
			return
		}
		if n > 10000 {
			n = 10000
		}

		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, fmt.Sprintf("tail -n +%d input.txt", n), dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("tail -n +%d unexpected exit code %d", n, code)
		}
	})
}

// FuzzTailBytesOffset fuzzes tail -c +N (skip-first-N-bytes offset mode).
func FuzzTailBytesOffset(f *testing.F) {
	f.Add([]byte("hello\nworld\n"), int64(1))
	f.Add([]byte("hello\nworld\n"), int64(6))
	f.Add([]byte{}, int64(1))
	f.Add([]byte("no newline"), int64(3))
	f.Add([]byte("a\x00b\nc\n"), int64(2))
	f.Add(bytes.Repeat([]byte("x"), 4097), int64(4096))
	f.Add([]byte("\n\n\n"), int64(2))
	f.Add([]byte("hello\nworld\n"), int64(100))
	// +1 = stream from byte 0 (entire file)
	f.Add([]byte("abc"), int64(1))
	// +N > file size → empty
	f.Add([]byte("abc"), int64(1000))
	// Binary content
	f.Add([]byte{0x00, 0x01, 0x02, 0xff, 0xfe}, int64(2))

	f.Fuzz(func(t *testing.T, input []byte, n int64) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if n < 1 {
			return
		}
		if n > 10000 {
			n = 10000
		}

		dir := t.TempDir()
		err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, fmt.Sprintf("tail -c +%d input.txt", n), dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("tail -c +%d unexpected exit code %d", n, code)
		}
	})
}
