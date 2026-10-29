// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Fuzz harness for the xargs builtin. The seed corpus is derived from
// three independent sources, per `implement-posix-command` skill:
//
//  1. Implementation edge cases (token cap, default-mode separators,
//     replace-string semantics, command-line size).
//  2. Security history — common DoS / overflow / encoding-bug input
//     classes for command-builder tools (very-long argv, embedded NULs,
//     CRLF, invalid UTF-8, large -n values).
//  3. Existing test coverage — every distinct input shape used in
//     `builtins/xargs/xargs_test.go` and `tests/scenarios/cmd/xargs/`
//     also appears here so unit-test regressions cannot escape.
//
// Acceptable exit codes: 0 (success) and 1 (usage error). The xargs sub-
// command-failure codes (123/124/125) propagate from sub-builtin exit
// codes and are also legitimate. Any other code or a panic fails the test.
package xargs_test

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

// allowedXargsExit returns true for any exit code xargs can legitimately
// produce — 0 / 1 / 123 / 124 / 125 — and false otherwise.
func allowedXargsExit(code int) bool {
	switch code {
	case 0, 1, 123, 124, 125:
		return true
	}
	return false
}

// FuzzXargsDefault fuzzes the default-mode pipeline (whitespace tokens,
// quoting, escapes) by piping arbitrary content through `xargs echo`.
func FuzzXargsDefault(f *testing.F) {
	// --- Source A: implementation edge cases ---
	f.Add([]byte(""))                      // empty input — runs once with no args
	f.Add([]byte("a"))                     // single token, no newline
	f.Add([]byte("a\n"))                   // trailing newline
	f.Add([]byte("a b c\n"))               // multiple tokens, one line
	f.Add([]byte("a\nb\nc\n"))             // one token per line
	f.Add([]byte("'a b' c\n"))             // single-quoted group
	f.Add([]byte(`"a b" c` + "\n"))        // double-quoted group
	f.Add([]byte(`a\ b c` + "\n"))         // backslash-escaped space
	f.Add([]byte("\n\n\n"))                // only blank lines
	f.Add([]byte("   \t   \n"))            // only whitespace
	f.Add([]byte("a\tb\tc\n"))             // tab-separated
	f.Add([]byte("a\rb\rc\n"))             // CR-only
	f.Add([]byte("a\r\nb\r\n"))            // CRLF
	f.Add([]byte("a\vb\fc\n"))             // VT and FF
	f.Add(bytes.Repeat([]byte("x"), 4097)) // > read chunk
	// --- Source B: security history ---
	f.Add(bytes.Repeat([]byte{'a'}, (1<<20)-1))             // just under MaxTokenBytes
	f.Add(bytes.Repeat([]byte{'a'}, (1<<20)+10))            // just over MaxTokenBytes
	f.Add([]byte("a\x00b\x00c\n"))                          // embedded NULs in default mode
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'}) // overlong UTF-8
	f.Add([]byte{0xed, 0xa0, 0x80, '\n'})                   // surrogate
	f.Add([]byte{0x80, '\n'})                               // bare continuation byte
	f.Add([]byte("a\\\nb\n"))                               // backslash-newline (line continuation)
	f.Add([]byte("'unterminated\n"))                        // unterminated single quote
	f.Add([]byte(`"unterminated`))                          // unterminated double quote at EOF
	f.Add([]byte(`a\`))                                     // trailing backslash at EOF
	// --- Source C: existing tests ---
	f.Add([]byte("alpha\nbeta\ngamma\n"))
	f.Add([]byte("a b c d e\n"))
	f.Add([]byte("a b STOP c d\n")) // EOF marker placeholder

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20+100 {
			return // bound input size
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "in.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, _, code := cmdRunCtxFuzz(ctx, t, "xargs -a in.txt echo > /dev/null", dir)
		if t.Context().Err() != nil {
			return
		}
		if !allowedXargsExit(code) {
			t.Errorf("xargs default unexpected exit code %d", code)
		}
	})
}

// FuzzXargsNullSeparated fuzzes -0 (null-separated, no quoting).
func FuzzXargsNullSeparated(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte{0})
	f.Add([]byte("a\x00"))
	f.Add([]byte("a\x00b\x00c\x00"))
	f.Add([]byte("a b\x00c d\x00")) // whitespace literal in -0 mode
	f.Add([]byte("a"))              // trailing token, no NUL
	f.Add(append(bytes.Repeat([]byte{'x'}, 4097), 0))
	f.Add(append(bytes.Repeat([]byte{'x'}, (1<<20)+10), 0)) // over cap
	f.Add([]byte{0xff, 0, 0xfe, 0})                         // high bytes
	f.Add([]byte{0xfc, 0x80, 0x80, 0})                      // bad UTF-8
	f.Add([]byte("a\nb\x00"))                               // newline literal in -0 mode

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20+100 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "in.bin"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, _, code := cmdRunCtxFuzz(ctx, t, "xargs -0 -a in.bin echo > /dev/null", dir)
		if t.Context().Err() != nil {
			return
		}
		if !allowedXargsExit(code) {
			t.Errorf("xargs -0 unexpected exit code %d", code)
		}
	})
}

// FuzzXargsReplace fuzzes -I (line-oriented, REPLSTR substitution).
func FuzzXargsReplace(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("a\n"))
	f.Add([]byte("a\nb\nc\n"))
	f.Add([]byte("a b c\n"))    // whole line is one item
	f.Add([]byte("a\n\n\nb\n")) // blank lines skipped
	f.Add([]byte("a"))          // no trailing newline
	f.Add([]byte("\n\n\n"))     // only blanks
	f.Add(append(bytes.Repeat([]byte{'x'}, 4097), '\n'))
	f.Add(append(bytes.Repeat([]byte{'x'}, (1<<20)+10), '\n')) // over cap
	f.Add([]byte("a\r\nb\r\n"))                                // CRLF
	f.Add([]byte("with $special chars\n"))                     // shell metas (no shell in loop)

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20+100 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "in.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, _, code := cmdRunCtxFuzz(ctx, t, "xargs -I {} -a in.txt echo {} > /dev/null", dir)
		if t.Context().Err() != nil {
			return
		}
		if !allowedXargsExit(code) {
			t.Errorf("xargs -I unexpected exit code %d", code)
		}
	})
}

// FuzzXargsDelimiter fuzzes -d C (single custom delimiter, no quoting).
func FuzzXargsDelimiter(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte(","))
	f.Add([]byte("a,"))
	f.Add([]byte("a,b,c,"))
	f.Add([]byte("a,b,c"))    // trailing token without separator
	f.Add([]byte("a b,c d,")) // whitespace literal
	f.Add(append(bytes.Repeat([]byte{'x'}, 4097), ','))
	f.Add(append(bytes.Repeat([]byte{'x'}, (1<<20)+10), ',')) // over cap
	f.Add([]byte{0xff, ',', 0xfe, ','})

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20+100 {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "in.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, _, code := cmdRunCtxFuzz(ctx, t, "xargs -d , -a in.txt echo > /dev/null", dir)
		if t.Context().Err() != nil {
			return
		}
		if !allowedXargsExit(code) {
			t.Errorf("xargs -d unexpected exit code %d", code)
		}
	})
}

// FuzzXargsBatching fuzzes the -n / -L / -s batching code paths over
// arbitrary content + arbitrary integer parameters.
func FuzzXargsBatching(f *testing.F) {
	f.Add([]byte(""), 1, 1, 16384)
	f.Add([]byte("a b c d e\n"), 2, 0, 16384)
	f.Add([]byte("a\nb\nc\n"), 0, 1, 16384)
	f.Add([]byte("a b\nc d\ne f\n"), 0, 2, 16384)
	f.Add([]byte("a b c\n"), 0, 0, 1) // tiny -s budget
	f.Add([]byte("alpha beta gamma\n"), 1, 0, 32)
	f.Add([]byte("alpha beta gamma\n"), 9999999, 0, 32) // n > HardMaxArgs
	f.Add([]byte("alpha beta gamma\n"), 0, 9999999, 32) // L > HardMaxArgs

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte, n, l, s int) {
		if t.Context().Err() != nil {
			return
		}
		if len(input) > 1<<20+100 {
			return
		}
		if n < 0 || l < 0 || s < 0 {
			return // negatives are tested as separate validation cases
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "in.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		// Build an xargs invocation that toggles each flag iff the value
		// is non-zero. -s is always set (must be > 0).
		script := "xargs -a in.txt"
		if n > 0 {
			script += " -n " + itoa(n)
		}
		if l > 0 {
			script += " -L " + itoa(l)
		}
		if s > 0 {
			script += " -s " + itoa(s)
		}
		script += " echo > /dev/null"

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, _, code := cmdRunCtxFuzz(ctx, t, script, dir)
		if t.Context().Err() != nil {
			return
		}
		if !allowedXargsExit(code) {
			t.Errorf("xargs batching unexpected exit code %d (script=%s)", code, script)
		}
	})
}

// itoa is a tiny strconv replacement to avoid the strconv import (not on
// the per-command allowlist for `xargs` test code; here we are inside a
// _test package so the allowlist does not apply, but keeping it minimal
// matches existing fuzz tests).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	v := n
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
