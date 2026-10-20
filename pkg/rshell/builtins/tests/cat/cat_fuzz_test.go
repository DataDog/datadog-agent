// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cat_test

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

// FuzzCat fuzzes cat with arbitrary file content and verifies output equals input.
func FuzzCat(f *testing.F) {
	// Basic
	f.Add([]byte("hello\nworld\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("\n\n\n"))
	// Null bytes — passed through unchanged (binary safety)
	f.Add([]byte("a\x00b\n"))
	f.Add([]byte{0x00, 0x00, 0x00})
	// High bytes / non-UTF-8 (M- notation only in -v mode; raw pass-through here)
	f.Add([]byte{0xff, 0xfe, 0x00, 0x01})
	f.Add([]byte{0x80, 0x9f, 0xa0, 0xfe, 0xff, '\n'})
	// Invalid UTF-8 sequences (CVE-class: must not crash on bad encoding)
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'})
	f.Add([]byte{0xed, 0xa0, 0x80}) // surrogate half
	// CRLF — must be preserved exactly
	f.Add([]byte("line1\r\nline2\r\n"))
	f.Add([]byte("a\r\nb\n"))
	// Near scanner buffer boundaries (init=4096, max=1MiB)
	f.Add(bytes.Repeat([]byte("x"), 4095))
	f.Add(bytes.Repeat([]byte("x"), 4096))
	f.Add(bytes.Repeat([]byte("x"), 4097))
	// Lines near the 1 MiB cap
	f.Add(append(bytes.Repeat([]byte("a"), 1<<20-1), '\n'))
	// DEL and other control chars
	f.Add([]byte{0x7f, '\n'})
	f.Add([]byte{0x01, 0x1f, 0x7f, '\n'})
	// Mixed binary and text
	f.Add([]byte("text\x00\x01\x02more text\n"))
	// ANSI/terminal escape sequences (terminal injection class — cat passes through unchanged)
	f.Add([]byte("\x1b[31mRED\x1b[0m\n"))         // ANSI color codes
	f.Add([]byte("\x1b]2;malicious title\x07\n")) // OSC 2: terminal title injection
	f.Add([]byte("\x1b[2J\n"))                    // clear screen
	f.Add([]byte("\x1b[9D\n"))                    // cursor back 9 columns
	f.Add([]byte("\x1bP...string...\x1b\\\n"))    // DCS device control sequence
	f.Add([]byte("\x1b]50;fontname\x07\n"))       // OSC 50 font query (xterm CVE class)
	// Bare CR (old Mac line endings)
	f.Add([]byte("a\rb\rc\r"))
	// ELF magic bytes (binary format detection)
	f.Add([]byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00})

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
		stdout, _, code := cmdRunCtx(ctx, t, "cat input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d", code)
		}

		// cat must output exactly the file contents
		if code == 0 && stdout != string(input) {
			t.Errorf("cat output differs from input: got %d bytes, want %d bytes", len(stdout), len(input))
		}
	})
}

// FuzzCatNumberLines fuzzes cat -n with arbitrary file content.
// Edge cases: line number formatting at width 6, blank lines, no trailing newline.
func FuzzCatNumberLines(f *testing.F) {
	f.Add([]byte("line1\nline2\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\nc\n"))
	f.Add([]byte("\n\n\n"))
	// Lines at scanner cap boundary — should error, not panic
	f.Add(append(bytes.Repeat([]byte("a"), 1<<20), '\n'))   // over cap: error
	f.Add(append(bytes.Repeat([]byte("b"), 1<<20-1), '\n')) // just under cap: ok
	// Blank-line interactions
	f.Add([]byte("a\n\n\nb\n"))
	// CRLF must be preserved
	f.Add([]byte("a\r\nb\r\n"))
	// Null bytes in line
	f.Add([]byte("x\x00y\nz\n"))
	// High bytes in line
	f.Add([]byte{0x80, 0x81, '\n'})

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
		_, _, code := cmdRunCtx(ctx, t, "cat -n input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("cat -n unexpected exit code %d", code)
		}
	})
}

// FuzzCatDisplayFlags fuzzes cat with display-transformation flags (-v/-E/-T/-A).
// Edge cases: M- notation for high bytes, ^X notation for controls, CRLF rendering.
func FuzzCatDisplayFlags(f *testing.F) {
	// Non-printing chars: must render as ^X
	f.Add([]byte{0x00, 0x01, 0x1f, '\n'}, true, false, false)
	// DEL → ^?
	f.Add([]byte{0x7f, '\n'}, true, false, false)
	// High bytes 0x80-0xff → M- notation
	f.Add([]byte{0x80, 0x9f, 0xa0, 0xff, '\n'}, true, false, false)
	// Tab handling: -v preserves tab, -T converts to ^I
	f.Add([]byte("a\tb\n"), true, false, false)
	f.Add([]byte("a\tb\n"), false, false, true)
	// -E: CRLF → ^M$ before the newline
	f.Add([]byte("a\r\nb\n"), false, true, false)
	// Combined -v -E: both transformations
	f.Add([]byte{0x00, '\r', '\n'}, true, true, false)
	// Empty lines with -E → just "$\n"
	f.Add([]byte("\n\n\n"), false, true, false)
	// Null bytes with -v
	f.Add([]byte{0x00, 0x00, '\n'}, true, false, false)
	// Surrogate / bad UTF-8 with -v
	f.Add([]byte{0xed, 0xa0, 0x80, '\n'}, true, false, false)

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte, flagV, flagE, flagT bool) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if !flagV && !flagE && !flagT {
			return // plain cat is covered by FuzzCat
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.bin"), input, 0644); err != nil {
			t.Fatal(err)
		}

		flags := ""
		if flagV {
			flags += " -v"
		}
		if flagE {
			flags += " -E"
		}
		if flagT {
			flags += " -T"
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "cat"+flags+" input.bin", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("cat%s unexpected exit code %d", flags, code)
		}
	})
}

// FuzzCatStdin fuzzes cat reading from stdin via shell redirection.
func FuzzCatStdin(f *testing.F) {
	f.Add([]byte("hello\nworld\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\n"))
	f.Add(bytes.Repeat([]byte("x"), 4097))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte{0xff, 0xfe, 0x00, 0x01})
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'})
	f.Add([]byte("line1\r\nline2\r\n"))

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

		err := os.WriteFile(filepath.Join(dir, "stdin.txt"), input, 0644)
		if err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		stdout, _, code := cmdRunCtx(ctx, t, "cat < stdin.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("cat stdin unexpected exit code %d", code)
		}

		if code == 0 && stdout != string(input) {
			t.Errorf("cat stdin output differs from input: got %d bytes, want %d bytes", len(stdout), len(input))
		}
	})
}
