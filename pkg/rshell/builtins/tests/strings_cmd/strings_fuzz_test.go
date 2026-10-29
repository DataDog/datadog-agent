// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package strings_cmd_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func cmdRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir, interp.AllowedPaths([]string{dir}))
}

// FuzzStrings fuzzes strings with arbitrary file content.
// Edge cases: isPrintable boundary bytes (0x1f not printable, 0x20 yes;
// 0x7e yes, 0x7f not; 0x09 tab yes, 0x0a newline not), defaultMinLen=4,
// maxStringLen=1 MiB cap, chunk boundary at 32 KiB.
func FuzzStrings(f *testing.F) {
	f.Add([]byte("hello world\x00\x01\x02binary\x00readable text\n"))
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x01, 0x02, 0x03})
	f.Add([]byte("all printable text\n"))
	f.Add(bytes.Repeat([]byte{0xff}, 4097))
	f.Add(bytes.Repeat([]byte("abcd"), 1024))
	f.Add([]byte("short\x00ab\x00longer string here\x00"))
	// isPrintable boundary: 0x1f (not printable) vs 0x20 (space, printable)
	f.Add([]byte{0x1f, 'a', 'b', 'c', 'd', 0x1f})
	f.Add([]byte{0x20, 'a', 'b', 'c', 'd', 0x20})
	// 0x7e (~) is printable, 0x7f (DEL) is not
	f.Add([]byte{0x7e, 'a', 'b', 'c', 'd', 0x7e})
	f.Add([]byte{0x7f, 'a', 'b', 'c', 'd', 0x7f})
	// 0x09 (tab) is printable, 0x08 (backspace) is not
	f.Add([]byte{'\t', 'a', 'b', 'c', 'd', '\t'})
	f.Add([]byte{0x08, 'a', 'b', 'c', 'd', 0x08})
	// Exactly 4 bytes (default minimum length — boundary)
	f.Add([]byte("abcd"))
	// Exactly 3 bytes (below minimum — should not print)
	f.Add([]byte("abc"))
	// maxStringLen: printable run at 1 MiB boundary (capped, then continues)
	f.Add(bytes.Repeat([]byte("x"), 1<<20-1))
	f.Add(bytes.Repeat([]byte("x"), 1<<20))
	f.Add(bytes.Repeat([]byte("x"), 1<<20+1))
	// Chunk boundary at 32 KiB: string spanning two chunks
	f.Add(append(bytes.Repeat([]byte("a"), 32*1024-2), []byte("bc\x00rest")...))
	// Alternating printable/non-printable
	f.Add(bytes.Repeat([]byte{'a', 0x00}, 100))
	// Only tab characters (printable)
	f.Add(bytes.Repeat([]byte{'\t'}, 10))
	// High bytes (all non-printable)
	f.Add(bytes.Repeat([]byte{0x80}, 100))
	f.Add(bytes.Repeat([]byte{0xff}, 100))
	// Null bytes as non-printable terminators
	f.Add([]byte{0x00, 'h', 'e', 'l', 'l', 'o', 0x00})
	// Mixed printable sequences of various lengths
	f.Add([]byte("ab\x00abc\x00abcd\x00abcde\x00"))
	// ELF magic bytes (CVE-2014-8485: crafted ELF triggers libbfd on old binutils;
	// our implementation scans raw bytes without libbfd, so no CVE exposure,
	// but good to confirm graceful handling of binary format magic numbers)
	f.Add([]byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00, 0x00, 0x00})
	// PE/COFF magic (Windows executables)
	f.Add([]byte{'M', 'Z', 0x90, 0x00, 0x03, 0x00, 0x00, 0x00})
	// ZIP magic
	f.Add([]byte{'P', 'K', 0x03, 0x04})
	// PDF magic with printable sequences inside
	f.Add([]byte("%PDF-1.4\x00\x00\x00binary\x00more text here\x00"))

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

		if err := os.WriteFile(filepath.Join(dir, "input.bin"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "strings input.bin", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("strings unexpected exit code %d", code)
		}
	})
}

// FuzzStringsMinLen fuzzes strings -n N with arbitrary file content and min length.
// Edge cases: n=1 (every single printable), n=maxStringLen (1 MiB),
// sequences exactly at boundary, below boundary.
func FuzzStringsMinLen(f *testing.F) {
	f.Add([]byte("hello world\x00\x01\x02binary\n"), int64(4))
	f.Add([]byte("ab\x00cdef\x00gh\n"), int64(1))
	f.Add([]byte("ab\x00cdef\x00gh\n"), int64(10))
	f.Add([]byte{}, int64(4))
	f.Add(bytes.Repeat([]byte("x"), 100), int64(50))
	// n=1: every printable byte reported individually
	f.Add([]byte("a\x00b\x00c\x00"), int64(1))
	// n=3 vs 4 (default): boundary between short/long sequences
	f.Add([]byte("abc\x00abcd\x00"), int64(3))
	f.Add([]byte("abc\x00abcd\x00"), int64(4))
	// Sequence exactly at minLen boundary
	f.Add([]byte("abcde\x00"), int64(5))
	f.Add([]byte("abcde\x00"), int64(6))
	// Large minLen: only very long sequences match
	f.Add(bytes.Repeat([]byte("x"), 1000), int64(999))
	f.Add(bytes.Repeat([]byte("x"), 1000), int64(1000))
	f.Add(bytes.Repeat([]byte("x"), 1000), int64(1001))
	// Tab as printable (contributes to sequence length)
	f.Add([]byte("ab\tcd\x00"), int64(4))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte, minLen int64) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if minLen < 1 || minLen > 1000 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.bin"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, fmt.Sprintf("strings -n %d input.bin", minLen), dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("strings -n %d unexpected exit code %d", minLen, code)
		}
	})
}

// FuzzStringsRadix fuzzes strings -t with offset radix formatting.
// Edge cases: 7-char field width (offsets > 9999999 overflow), large files,
// offsets at octal/decimal/hex field boundaries.
func FuzzStringsRadix(f *testing.F) {
	f.Add([]byte("hello\x00world\x00text\n"), "o")
	f.Add([]byte("hello\x00world\x00text\n"), "d")
	f.Add([]byte("hello\x00world\x00text\n"), "x")
	// Large offset: test multi-char field formatting at ~1 MiB boundary.
	// Octal 7777777 = 2097151 — seed just under 2 MiB exercises 7-digit octal.
	f.Add(append(bytes.Repeat([]byte{0x00}, 1<<20), []byte("hello")...), "o")
	// Decimal offset at ~1 MiB boundary (7-digit decimal: 1048576+)
	f.Add(append(bytes.Repeat([]byte{0x00}, 1<<20), []byte("abcde")...), "d")
	// Hex offset at ~1 MiB boundary (0x100000 = 6 hex digits)
	f.Add(append(bytes.Repeat([]byte{0x00}, 1<<20), []byte("hello")...), "x")
	// Empty input
	f.Add([]byte{}, "d")
	// All non-printable (no output)
	f.Add(bytes.Repeat([]byte{0x00}, 100), "x")
	// Multiple strings with increasing offsets
	f.Add([]byte("hello\x00world\x00foo\x00bar\x00"), "d")

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte, radix string) {
		if t.Context().Err() != nil {
			return
		}
		// Allow up to 2 MiB so the large-offset corpus seeds (~1 MiB) execute.
		if len(input) > 2<<20 {
			return
		}
		if radix != "o" && radix != "d" && radix != "x" {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.bin"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, fmt.Sprintf("strings -t %s input.bin", radix), dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("strings -t %s unexpected exit code %d", radix, code)
		}
	})
}

// FuzzStringsStdin fuzzes strings reading from stdin.
func FuzzStringsStdin(f *testing.F) {
	f.Add([]byte("hello\x00\x01\x02world\n"))
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x01, 0x02, 0x03})
	// Printable boundary bytes
	f.Add([]byte{0x1f, 'a', 'b', 'c', 'd', 0x20})
	f.Add([]byte{0x7e, 'a', 'b', 'c', 'd', 0x7f})
	// Tab printable
	f.Add([]byte{'\t', 'a', 'b', 'c', '\t'})
	// Chunk boundary
	f.Add(append(bytes.Repeat([]byte("a"), 32*1024-1), 0x00))

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

		if err := os.WriteFile(filepath.Join(dir, "stdin.bin"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "strings < stdin.bin", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("strings stdin unexpected exit code %d", code)
		}
	})
}
