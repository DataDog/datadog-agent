// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package grep_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func cmdRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir, interp.AllowedPaths([]string{dir}))
}

// FuzzGrepFileContent fuzzes grep with a fixed pattern and arbitrary file content.
// Edge cases: binary content, null bytes, lines at 1 MiB cap, invalid UTF-8.
func FuzzGrepFileContent(f *testing.F) {
	f.Add([]byte("apple\nbanana\ncherry\n"), "banana")
	f.Add([]byte{}, "anything")
	f.Add([]byte("no newline"), "new")
	f.Add([]byte("a\x00b\nc\n"), "a")
	f.Add(bytes.Repeat([]byte("x"), 4097), "x")
	f.Add([]byte("\n\n\n"), ".")
	f.Add([]byte("hello world\nfoo bar\n"), "foo")
	f.Add([]byte{0xff, 0xfe}, "a")
	// Lines at/over 1 MiB cap
	f.Add(append(bytes.Repeat([]byte("a"), 1<<20-1), '\n'), "a")
	f.Add(append(bytes.Repeat([]byte("a"), 1<<20), '\n'), "a")
	// CRLF
	f.Add([]byte("hello\r\nworld\r\n"), "hello")
	// Invalid UTF-8
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'}, "a")
	f.Add([]byte{0xed, 0xa0, 0x80, '\n'}, "a")
	// Null bytes in content
	f.Add([]byte{0x00, 0x00, '\n'}, "a")
	// BRE special chars in content being matched
	f.Add([]byte("a.b\na*b\na[b\n"), "a.b")
	f.Add([]byte("(test)\n[bracket]\n"), "test")
	// Word-boundary content
	f.Add([]byte("foo foobar barfoo\n"), "foo")
	// Multibyte content
	f.Add([]byte("héllo\nmünchen\n"), "l")

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte, pattern string) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if !utf8.ValidString(pattern) {
			return
		}
		for _, c := range pattern {
			if c == '\'' || c == '\x00' || c == '\n' {
				return
			}
			// C0/DEL/C1 control chars confuse the shell script parser.
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}
		if len(pattern) == 0 {
			return
		}
		if len(pattern) > 100 {
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
		script := "grep '" + pattern + "' input.txt"
		_, _, code := cmdRunCtx(ctx, t, script, dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("grep unexpected exit code %d", code)
		}
	})
}

// FuzzGrepPatterns fuzzes grep with arbitrary regex patterns on fixed content.
// Edge cases: BRE→ERE conversion, pathological backtracking patterns, empty patterns.
func FuzzGrepPatterns(f *testing.F) {
	// BRE special chars
	f.Add([]byte("hello world\nfoo bar\n"), "hel+o")
	f.Add([]byte("aaa\nbbb\n"), "a*")
	f.Add([]byte("test123\n"), "[0-9]+")
	f.Add([]byte("(parens)\n"), "[(]")
	// Anchors
	f.Add([]byte("hello\nworld\n"), "^hello")
	f.Add([]byte("hello\nworld\n"), "world$")
	f.Add([]byte("hello\n"), "^hello$")
	// Pathological backtracking patterns (ReDoS class)
	f.Add([]byte("aaaaaaaaaaaaaab\n"), "a*a*b")
	f.Add([]byte("aaaaaaaaaaaaaaaa\n"), "(a+)+")
	// BRE escaping: \( is group in BRE; ( is literal
	f.Add([]byte("(test)\n"), "\\(test\\)")
	// Dot matches everything except newline
	f.Add([]byte("abc\n"), ".")
	f.Add([]byte("\n"), ".")
	// Character classes
	f.Add([]byte("hello123\n"), "[[:alpha:]]")
	f.Add([]byte("hello123\n"), "[[:digit:]]")
	f.Add([]byte("HELLO\n"), "[[:upper:]]")
	// Empty match
	f.Add([]byte("hello\n"), "")
	// Very long pattern
	f.Add([]byte("aaaa\n"), "a{1,4}")

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte, pattern string) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if !utf8.ValidString(pattern) {
			return
		}
		for _, c := range pattern {
			if c == '\'' || c == '\x00' || c == '\n' {
				return
			}
			// C0/DEL/C1 control chars confuse the shell script parser.
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}
		if len(pattern) > 100 {
			return
		}
		if len(pattern) == 0 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "grep '"+pattern+"' input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("grep pattern %q unexpected exit code %d", pattern, code)
		}
	})
}

// FuzzGrepStdin fuzzes grep reading from stdin with arbitrary content.
func FuzzGrepStdin(f *testing.F) {
	f.Add([]byte("apple\nbanana\ncherry\n"))
	f.Add([]byte{})
	f.Add([]byte("no newline"))
	f.Add([]byte("a\x00b\nc\n"))
	f.Add(bytes.Repeat([]byte("x"), 4097))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'})
	f.Add([]byte("line1\r\nline2\r\n"))
	f.Add(append(bytes.Repeat([]byte("a"), 1<<20-1), '\n'))

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
		_, _, code := cmdRunCtx(ctx, t, "grep '.' < stdin.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("grep stdin unexpected exit code %d", code)
		}
	})
}

// FuzzGrepFixedStrings fuzzes grep -F (fixed string mode) with arbitrary content and patterns.
// CVE-2015-1345 affected bmexec_trans in kwset.c when using -F; out-of-bounds heap read
// triggered by crafted input+pattern combinations in Boyer-Moore-Horspool matching.
// CVE-2012-5667 was an integer overflow triggered by lines >= 2^31 bytes (we cap at 1 MiB).
func FuzzGrepFixedStrings(f *testing.F) {
	f.Add([]byte("hello world\nfoo bar\n"), "hello")
	f.Add([]byte{}, "pattern")
	f.Add([]byte("no newline"), "no")
	f.Add([]byte("a\x00b\nc\n"), "a")
	// Patterns that look like regex metacharacters (treated as literals with -F)
	f.Add([]byte("(parens)\n[bracket]\na.b\na*b\n"), "(parens)")
	f.Add([]byte("(parens)\n[bracket]\na.b\na*b\n"), "[bracket]")
	f.Add([]byte("a.b\naab\n"), "a.b")  // dot is literal, not wildcard
	f.Add([]byte("a*b\nab\n"), "a*b")   // star is literal, not quantifier
	f.Add([]byte("a+b\nab\n"), "a+b")   // plus is literal
	f.Add([]byte("a?b\nab\n"), "a?b")   // question mark is literal
	f.Add([]byte("^start\n"), "^start") // caret is literal
	f.Add([]byte("end$\n"), "end$")     // dollar is literal
	// Backslash in pattern (treated as literal with -F)
	f.Add([]byte("a\\b\nab\n"), "a\\b")
	// Empty pattern match
	f.Add([]byte("hello\nworld\n"), "")
	// Binary content with printable pattern
	f.Add([]byte{0xff, 0xfe, 'h', 'i', '\n'}, "hi")
	// CRLF
	f.Add([]byte("hello\r\nworld\r\n"), "hello")
	// Invalid UTF-8
	f.Add([]byte{0xfc, 0x80, 0x80, 'h', 'i', '\n'}, "hi")
	// Near 1 MiB line cap (CVE-2012-5667 was 2^31; we test our 1 MiB boundary)
	f.Add(append(bytes.Repeat([]byte("a"), 1<<20-1), '\n'), "a")

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte, pattern string) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if !utf8.ValidString(pattern) {
			return
		}
		if len(pattern) > 100 {
			return
		}
		for _, c := range pattern {
			if c == '\'' || c == '\x00' || c == '\n' {
				return
			}
			// C0/DEL/C1 control chars confuse the shell script parser.
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := cmdRunCtx(ctx, t, "grep -F '"+pattern+"' input.txt", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("grep -F unexpected exit code %d", code)
		}
	})
}

// FuzzGrepFlags fuzzes grep with various flag combinations and arbitrary file content.
// Edge cases: context line clamping (MaxContextLines=1000), -q early exit, -o empty match.
func FuzzGrepFlags(f *testing.F) {
	f.Add([]byte("Hello\nworld\nHELLO\n"), true, false, false, false, int64(0), int64(0))
	f.Add([]byte("line1\nline2\n"), false, true, false, false, int64(0), int64(0))
	f.Add([]byte{}, true, true, false, false, int64(0), int64(0))
	f.Add([]byte("no newline"), false, false, false, false, int64(0), int64(0))
	f.Add(bytes.Repeat([]byte("abc\n"), 100), true, false, false, false, int64(0), int64(0))
	// Context lines
	f.Add([]byte("a\nb\nc\nd\ne\n"), false, false, false, false, int64(2), int64(0))
	f.Add([]byte("a\nb\nc\nd\ne\n"), false, false, false, false, int64(0), int64(2))
	// Context clamping at MaxContextLines=1000
	f.Add([]byte("a\nb\n"), false, false, false, false, int64(1001), int64(0))
	// -c (count) mode
	f.Add([]byte("a\na\nb\n"), false, false, true, false, int64(0), int64(0))
	// -q (quiet) mode: exits on first match
	f.Add([]byte("a\nb\nc\n"), false, false, false, true, int64(0), int64(0))
	// Binary content
	f.Add([]byte{0xff, 0xfe, '\n'}, true, false, false, false, int64(0), int64(0))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte, caseInsensitive, invertMatch, countOnly, quiet bool, afterCtx, beforeCtx int64) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20 {
			return
		}
		if afterCtx < 0 || afterCtx > 100 {
			return
		}
		if beforeCtx < 0 || beforeCtx > 100 {
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
		flags := ""
		if caseInsensitive {
			flags += " -i"
		}
		if invertMatch {
			flags += " -v"
		}
		if countOnly {
			flags += " -c"
		}
		if quiet {
			flags += " -q"
		}
		if afterCtx > 0 {
			flags += " -A " + fmt.Sprintf("%d", afterCtx)
		}
		if beforeCtx > 0 {
			flags += " -B " + fmt.Sprintf("%d", beforeCtx)
		}

		script := "grep" + flags + " 'a' input.txt"
		_, _, code := cmdRunCtx(ctx, t, script, dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("grep%s unexpected exit code %d", flags, code)
		}
	})
}
