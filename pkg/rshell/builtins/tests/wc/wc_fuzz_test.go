// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package wc_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
)

// FuzzWc fuzzes wc (default mode: lines, words, bytes) with arbitrary file content.
// Edge cases: UTF-8 chunk boundary carry-over, wide chars, tab stops, CRLF.
func FuzzWc(f *testing.F) {
	f.Add([]byte("hello world\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\nc\n"))
	f.Add(bytes.Repeat([]byte("x"), 4097))
	f.Add([]byte("\n\n\n"))
	f.Add(bytes.Repeat([]byte("word "), 100))
	// Tab stops: wc -L counts tab as advancing to next 8-column boundary
	f.Add([]byte("a\tb\tc\n"))
	f.Add([]byte("\t\t\t\n"))
	// CRLF: \r resets word state without starting newline
	f.Add([]byte("a\r\nb\r\n"))
	f.Add([]byte("word1\r\nword2\r\n"))
	// Multibyte UTF-8: wc -m counts runes; wc -c counts bytes
	f.Add([]byte("héllo\n")) // 2-byte é
	f.Add([]byte("日本語\n"))   // 3-byte CJK
	f.Add([]byte("😀\n"))     // 4-byte emoji
	f.Add([]byte("こんにちは\n")) // wide chars (width 2 each for -L)
	// UTF-8 split at 32 KiB chunk boundary (carry-over bytes logic)
	f.Add(append(bytes.Repeat([]byte("a"), 32*1024-1), []byte("é")...))
	// Invalid UTF-8 (must not crash — processed as replacement char)
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'})
	f.Add([]byte{0xed, 0xa0, 0x80, '\n'}) // surrogate
	f.Add([]byte{0x80, '\n'})             // continuation byte without lead
	// Null bytes
	f.Add([]byte{0x00, 0x00, '\n'})
	// High bytes
	f.Add([]byte{0x80, 0x9f, 0xa0, 0xff, '\n'})
	// Only whitespace
	f.Add([]byte("   \t   \n"))
	f.Add([]byte("\n\n\n\n\n"))
	// Long line (tests -L max-line-length tracking)
	f.Add(append(bytes.Repeat([]byte("a"), 1000), '\n'))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "wc input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("wc unexpected exit code %d", code)
		}
	})
}

// FuzzWcLines fuzzes wc -l with arbitrary file content.
func FuzzWcLines(f *testing.F) {
	f.Add([]byte("line1\nline2\nline3\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\nc\n"))
	f.Add(bytes.Repeat([]byte("x"), 4097))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte("line1\r\nline2\r\n"))
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'})
	f.Add(bytes.Repeat([]byte("a\n"), 10000))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "wc -l input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("wc -l unexpected exit code %d", code)
		}
	})
}

// FuzzWcBytes fuzzes wc -c with arbitrary file content.
func FuzzWcBytes(f *testing.F) {
	f.Add([]byte("hello\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\nc\n"))
	f.Add(bytes.Repeat([]byte("x"), 4097))
	f.Add([]byte{0x00, 0x01, 0x02, 0xff, 0xfe})
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf})

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "wc -c input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("wc -c unexpected exit code %d", code)
		}
	})
}

// FuzzWcChars fuzzes wc -m (character/rune count) with multibyte and invalid UTF-8.
// Edge cases: carry-over bytes at chunk boundaries, replacement chars for bad sequences.
func FuzzWcChars(f *testing.F) {
	f.Add([]byte("hello\n"))
	f.Add([]byte("héllo\n"))
	f.Add([]byte("日本語\n"))
	f.Add([]byte("😀\n"))
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'})
	f.Add([]byte{0x80, '\n'})
	f.Add([]byte{0xed, 0xa0, 0x80, '\n'})
	// Chunk boundary split: 3-byte rune straddling 32 KiB boundary
	f.Add(append(bytes.Repeat([]byte("a"), 32*1024-1), []byte("日")...))
	// 4-byte emoji straddling boundary
	f.Add(append(bytes.Repeat([]byte("a"), 32*1024-1), []byte("😀")...))
	f.Add([]byte{})
	f.Add([]byte("no newline"))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "wc -m input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("wc -m unexpected exit code %d", code)
		}
	})
}

// FuzzWcStdin fuzzes wc reading from stdin via shell redirection.
func FuzzWcStdin(f *testing.F) {
	f.Add([]byte("hello world\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\n"))
	f.Add(bytes.Repeat([]byte("x"), 4097))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte("héllo\n"))
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'})

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		// Keep the limit modest: stdin redirection goes through the full
		// shell interpreter pipeline, which is slower than direct file
		// reads. A 64 KiB cap prevents CI timeouts on resource-constrained
		// runners while still exercising interesting edge cases.
		if len(input) > 64*1024 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		err := os.WriteFile(filepath.Join(dir, "stdin.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "wc < stdin.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("wc stdin unexpected exit code %d", code)
		}
	})
}
