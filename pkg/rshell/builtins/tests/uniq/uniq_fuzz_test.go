// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package uniq_test

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
)

// fuzzRunCtx delegates to testutil.FuzzRunScriptCtx which runs the script
// with AllowedPaths set to [dir] for proper file access in fuzz iterations.
func fuzzRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.FuzzRunScriptCtx(ctx, t, script, dir)
}

// FuzzUniq fuzzes uniq with arbitrary file content.
// Edge cases: MaxLineBytes (1 MiB) cap, no-trailing-newline, null bytes, CRLF.
func FuzzUniq(f *testing.F) {
	f.Add([]byte("a\na\nb\nb\nc\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\nc\n"))
	f.Add(bytes.Repeat([]byte("x\n"), 100))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte("AAA\naaa\nAAA\n"))
	// All identical lines
	f.Add(bytes.Repeat([]byte("same\n"), 1000))
	// All unique lines
	f.Add([]byte("a\nb\nc\nd\ne\n"))
	// Single line, no newline
	f.Add([]byte("single"))
	// CRLF lines
	f.Add([]byte("a\r\na\r\nb\r\n"))
	// Lines near the 1 MiB cap
	f.Add(append(bytes.Repeat([]byte("a"), 1<<20-1), '\n'))
	f.Add(append(bytes.Repeat([]byte("a"), 1<<20), '\n'))
	// Null bytes in lines
	f.Add([]byte("a\x00b\na\x00b\nc\n"))
	// Invalid UTF-8
	f.Add([]byte{0xfc, 0x80, 0x80, '\n', 0xfc, 0x80, 0x80, '\n'})
	// countFieldWidth=7: count > 9999999 would overflow field
	f.Add(bytes.Repeat([]byte("x\n"), 10000000/2))
	// CVE-2013-0222 pattern: long line with embedded null bytes followed by CRLF.
	// The SUSE i18n patch used alloca() sized by line length → stack overflow at 50MB.
	// Our implementation uses fixed buffers; test at our MaxLineBytes (1 MiB) boundary.
	f.Add(append(append([]byte("1"), bytes.Repeat([]byte{0x00}, 1<<19)...), '\n'))
	// CRLF duplicate detection: lines identical except for trailing \r
	f.Add([]byte("a\r\na\r\n"))
	f.Add([]byte("a\r\na\n")) // CRLF vs LF — how are these compared?

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

		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := fuzzRunCtx(ctx, t, "uniq input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("uniq unexpected exit code %d", code)
		}
	})
}

// FuzzUniqCount fuzzes uniq -c with arbitrary file content.
// Edge cases: countFieldWidth=7, very large repeat counts, overflow formatting.
func FuzzUniqCount(f *testing.F) {
	f.Add([]byte("a\na\nb\nb\nc\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("a\na\na\n"))
	// Many duplicates — count field must not overflow
	f.Add(bytes.Repeat([]byte("x\n"), 9999998))
	// Single occurrence
	f.Add([]byte("unique\n"))
	// CRLF
	f.Add([]byte("a\r\na\r\nb\r\n"))

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

		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := fuzzRunCtx(ctx, t, "uniq -c input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("uniq -c unexpected exit code %d", code)
		}
	})
}

// FuzzUniqFlags fuzzes uniq with various flag combinations.
// Edge cases: -f/-s/-w field/char skipping with MaxCount clamp, -i case folding,
// -D/-d deduplication modes, -z NUL delimiter.
func FuzzUniqFlags(f *testing.F) {
	f.Add([]byte("a\na\nb\nb\nc\n"), true, false, false, false, int64(0), int64(0), int64(0))
	f.Add([]byte("AAA\naaa\nAAA\n"), false, true, false, false, int64(0), int64(0), int64(0))
	f.Add([]byte("  a x\n  a y\n  b x\n"), false, false, false, false, int64(1), int64(0), int64(0))
	f.Add([]byte("aaa\naab\naac\n"), false, false, false, false, int64(0), int64(2), int64(0))
	f.Add([]byte("a\na\nb\n"), false, false, true, false, int64(0), int64(0), int64(0))
	// -w with skip
	f.Add([]byte("abc123\nabc456\ndef\n"), false, false, false, false, int64(0), int64(0), int64(3))
	// -z NUL delimiter
	f.Add([]byte("a\x00a\x00b\x00"), false, false, false, true, int64(0), int64(0), int64(0))
	// MaxCount clamp: skipFields/skipChars/checkChars at int32 max
	f.Add([]byte("a b c\na b c\n"), false, false, false, false, int64(1<<31-1), int64(0), int64(0))
	f.Add([]byte("abcdef\nabcdef\n"), false, false, false, false, int64(0), int64(1<<31-1), int64(0))
	// -f large value (beyond any line): all lines unique
	f.Add([]byte("a b\na b\n"), false, false, false, false, int64(100), int64(0), int64(0))
	// -s large value: skips entire comparison key
	f.Add([]byte("abcdef\nabcdef\n"), false, false, false, false, int64(0), int64(100), int64(0))
	// -d: only print duplicate lines
	f.Add([]byte("a\na\nb\nc\nc\n"), true, false, false, false, int64(0), int64(0), int64(0))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte, repeated, ignoreCase, unique, nulDelim bool, skipFields, skipChars, checkChars int64) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if skipFields < 0 || skipFields > 100 {
			return
		}
		if skipChars < 0 || skipChars > 100 {
			return
		}
		if checkChars < 0 || checkChars > 100 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		flags := ""
		if repeated {
			flags += " -d"
		}
		if ignoreCase {
			flags += " -i"
		}
		if unique {
			flags += " -u"
		}
		if nulDelim {
			flags += " -z"
		}
		if skipFields > 0 {
			flags += fmt.Sprintf(" -f %d", skipFields)
		}
		if skipChars > 0 {
			flags += fmt.Sprintf(" -s %d", skipChars)
		}
		if checkChars > 0 {
			flags += fmt.Sprintf(" -w %d", checkChars)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := fuzzRunCtx(ctx, t, "uniq"+flags+" input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("uniq%s unexpected exit code %d", flags, code)
		}
	})
}

// FuzzUniqStdin fuzzes uniq reading from stdin.
func FuzzUniqStdin(f *testing.F) {
	f.Add([]byte("a\na\nb\nb\nc\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte{0xfc, 0x80, 0x80, '\n', 0xfc, 0x80, 0x80, '\n'})
	f.Add([]byte("line1\r\nline1\r\nline2\r\n"))

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

		if err := os.WriteFile(filepath.Join(dir, "stdin.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := fuzzRunCtx(ctx, t, "uniq < stdin.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("uniq stdin unexpected exit code %d", code)
		}
	})
}
