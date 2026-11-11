// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package echo_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
)

// fuzzRunCtx delegates to testutil.FuzzRunScriptCtx which runs the script
// with AllowedPaths set to [dir] for proper file access in fuzz iterations.
func fuzzRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.FuzzRunScriptCtx(ctx, t, script, dir)
}

// FuzzEcho fuzzes echo with arbitrary arguments (no escape processing).
func FuzzEcho(f *testing.F) {
	f.Add("hello world")
	f.Add("")
	f.Add("a\tb\tc")
	// Backslash passthrough (no -e, so \n is literal)
	f.Add("no\\nnewline")
	f.Add("back\\\\slash")
	// Unicode
	f.Add("héllo wörld")
	f.Add("日本語")
	f.Add("😀 emoji")
	// Long argument
	f.Add("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, arg string) {
		if t.Context().Err() != nil {
			return
		}
		if len(arg) > 1000 {
			return
		}
		if !utf8.ValidString(arg) {
			return
		}
		for _, c := range arg {
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

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := fuzzRunCtx(ctx, t, "echo '"+arg+"'", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 {
			t.Errorf("echo unexpected exit code %d", code)
		}
	})
}

// FuzzEchoEscapes fuzzes echo -e with arbitrary escape sequences.
// Edge cases: \c stops output, \0nnn octal, \xHH hex, \uHHHH unicode,
// surrogates replaced with U+FFFD, values >0x10FFFF silently dropped.
func FuzzEchoEscapes(f *testing.F) {
	f.Add("hello\\nworld")
	f.Add("\\t\\t\\t")
	// Hex escapes: \xHH (up to 2 hex digits)
	f.Add("\\x41\\x42\\x43") // "ABC"
	f.Add("\\x0")            // incomplete hex — outputs \x0 literally? no: valid 1-digit
	f.Add("\\xgg")           // invalid hex digits — outputs \x literally
	f.Add("\\x")             // no hex digits — outputs \x literally
	// Octal: \0nnn (up to 3 octal digits after 0)
	f.Add("\\0101") // 'A'
	f.Add("\\0377") // 0xff
	f.Add("\\0400") // > 0377: takes only 3 digits
	f.Add("\\08")   // 8 is not octal — stops after \00
	// Unicode: \uHHHH (4 hex) and \UHHHHHHHH (8 hex)
	f.Add("\\u0041")     // 'A'
	f.Add("\\u00e9")     // 'é'
	f.Add("\\u4e2d")     // '中'
	f.Add("\\uD800")     // surrogate — replaced with U+FFFD (intentional divergence from bash)
	f.Add("\\uDFFF")     // low surrogate — replaced with U+FFFD
	f.Add("\\U00010000") // first supplementary plane
	f.Add("\\U0010FFFF") // max valid codepoint
	f.Add("\\U00110000") // > max — silently dropped
	f.Add("\\UFFFFFFFF") // way over max — silently dropped
	// \c stops further output (including trailing newline)
	f.Add("hello\\cworld")
	f.Add("\\c")
	// Standard escapes
	f.Add("\\a\\b\\e\\E\\f\\r\\v")
	f.Add("\\\\") // literal backslash
	// Unrecognized escape: output backslash + char literally
	f.Add("\\q\\z\\j")
	// Mixed
	f.Add("tab:\\there\\nnewline:\\nend")
	// Long sequence to stress output buffering
	f.Add("\\n\\n\\n\\n\\n\\n\\n\\n\\n\\n")

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, arg string) {
		if t.Context().Err() != nil {
			return
		}
		if len(arg) > 1000 {
			return
		}
		if !utf8.ValidString(arg) {
			return
		}
		for _, c := range arg {
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

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := fuzzRunCtx(ctx, t, "echo -e '"+arg+"'", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 {
			t.Errorf("echo -e unexpected exit code %d", code)
		}
	})
}

// FuzzEchoFlagInteraction fuzzes echo with mixed -n/-e/-E flag combinations.
// Edge cases: last flag wins for -e/-E; -n suppresses trailing newline.
func FuzzEchoFlagInteraction(f *testing.F) {
	f.Add("hello", true, false, false)    // -n
	f.Add("hello\\n", false, true, false) // -e
	f.Add("hello\\n", false, false, true) // -E (disables escapes)
	f.Add("hi\\n", false, true, true)     // -e -E: -E wins (last)
	f.Add("hi\\n", true, true, false)     // -n -e

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, arg string, flagN, flagE, flagBigE bool) {
		if t.Context().Err() != nil {
			return
		}
		if len(arg) > 500 {
			return
		}
		if !utf8.ValidString(arg) {
			return
		}
		for _, c := range arg {
			if c == '\'' || c == '\x00' || c == '\n' {
				return
			}
			// C0/DEL/C1 control chars confuse the shell script parser.
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}

		flags := ""
		if flagN {
			flags += " -n"
		}
		if flagE {
			flags += " -e"
		}
		if flagBigE {
			flags += " -E"
		}
		if flags == "" {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		_, _, code := fuzzRunCtx(ctx, t, "echo"+flags+" '"+arg+"'", dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 {
			t.Errorf("echo%s unexpected exit code %d", flags, code)
		}
	})
}
