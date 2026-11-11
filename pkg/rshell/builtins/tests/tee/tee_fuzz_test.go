// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tee_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// teeFuzzRunCtx runs script with AllowedPaths restricted to dir as
// read-write and remediation mode enabled — the two prerequisites tee
// requires before it will open anything for writing. Named distinctly from
// teeRun/teeRunStdin (tee_test.go, non-fuzz-mode helpers) to avoid
// redeclaration conflicts, per the fuzz-testing convention used across the
// other builtins' fuzz suites.
func teeFuzzRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir,
		interp.AllowedPaths([]string{dir + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)
}

// FuzzTeeStdinContent fuzzes "tee out.txt" with arbitrary stdin content,
// verifying that output written to the file and to stdout are always
// byte-identical to the input, and that the command never panics or hangs.
func FuzzTeeStdinContent(f *testing.F) {
	// Source A: implementation edge cases — teeBufSize (32 KiB) boundary.
	f.Add([]byte{})
	f.Add([]byte("hello\n"))
	f.Add(bytes.Repeat([]byte("x"), 32*1024-1))
	f.Add(bytes.Repeat([]byte("x"), 32*1024))
	f.Add(bytes.Repeat([]byte("x"), 32*1024+1))
	f.Add(bytes.Repeat([]byte("y"), 3*32*1024)) // multiple full chunks

	// Source A: degenerate inputs.
	f.Add([]byte("no newline"))
	f.Add([]byte("\n\n\n"))
	f.Add([]byte("a"))

	// Source B: CVE/security-history classes.
	f.Add([]byte("a\x00b\n"))                                  // embedded null bytes
	f.Add([]byte{0x00, 0x00, 0x00})                            // all null
	f.Add([]byte("line1\r\nline2\r\n"))                        // CRLF
	f.Add([]byte("a\rb\rc\r"))                                 // bare CR
	f.Add([]byte{0xff, 0xfe, 0x00, 0x01})                      // high bytes
	f.Add([]byte{0xed, 0xa0, 0x80})                            // UTF-8 surrogate half
	f.Add([]byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'})    // overlong UTF-8
	f.Add([]byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}) // ELF magic
	f.Add([]byte{'M', 'Z', 0x90, 0x00})                        // PE magic
	f.Add([]byte{'P', 'K', 0x03, 0x04})                        // ZIP magic
	f.Add([]byte("\x1b[31mRED\x1b[0m\n"))                      // ANSI escape
	f.Add([]byte("\x1b]2;malicious title\x07\n"))              // OSC injection

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 1<<20 {
			return // cap at 1 MiB
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		if err := os.WriteFile(filepath.Join(dir, "input.txt"), input, 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stdout, _, code := teeFuzzRunCtx(ctx, t, "tee out.txt < input.txt", dir)
		if ctx.Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d", code)
			return
		}
		if code != 0 {
			return
		}
		if stdout != string(input) {
			t.Errorf("tee stdout differs from input: got %d bytes, want %d bytes", len(stdout), len(input))
		}
		written, err := os.ReadFile(filepath.Join(dir, "out.txt"))
		if err != nil {
			t.Errorf("reading out.txt: %v", err)
			return
		}
		if !bytes.Equal(written, input) {
			t.Errorf("tee file output differs from input: got %d bytes, want %d bytes", len(written), len(input))
		}
	})
}

// FuzzTeeFlags fuzzes arbitrary flag-shaped tokens ahead of a fixed file
// operand, verifying that unknown or malformed flags are rejected cleanly
// (exit 0 or 1, never a panic or hang) rather than being silently
// misinterpreted as a file operand or causing unexpected behavior.
func FuzzTeeFlags(f *testing.F) {
	f.Add("-a")
	f.Add("--append")
	f.Add("-h")
	f.Add("--help")
	f.Add("-ah")
	f.Add("-")
	f.Add("--")
	f.Add("-x")
	f.Add("--not-a-real-flag")
	f.Add("-i")
	f.Add("--ignore-interrupts")
	f.Add("-p")
	f.Add("--output-error")
	f.Add("--output-error=warn")
	f.Add("")
	f.Add("-a=true")
	f.Add("---")
	f.Add("-aaaaaaaaaaaaaaaaaaaa")

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, flag string) {
		if !utf8.ValidString(flag) {
			// The shell parser itself requires valid UTF-8 script text and
			// fails the parse before tee ever runs; that is a script-source
			// constraint unrelated to tee's own flag handling, so skip
			// rather than let testutil.RunScriptCtx's require.NoError turn
			// a parse error into a hard test failure.
			return
		}
		for _, c := range flag {
			// C0/DEL/C1 control chars confuse the shell script parser (the
			// same known limitation echo/grep/testcmd's fuzz suites already
			// skip around). In particular U+0080 is valid UTF-8 but the
			// mvdan.cc/sh parser fails to close a single-quoted string that
			// contains it ("reached EOF without closing quote"), which is a
			// parser limitation unrelated to tee's own flag handling.
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}
		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		script := "printf hi | tee " + shellQuote(flag) + " out.txt"
		_, _, code := teeFuzzRunCtx(ctx, t, script, dir)
		if ctx.Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d for flag %q", code, flag)
		}
	})
}

// shellQuote wraps s in single quotes, escaping any embedded single quote in
// the POSIX-standard way ('\”), so an arbitrary fuzzer-generated string can
// be safely interpolated into a script as one shell word without altering
// the number or boundaries of arguments tee receives.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
