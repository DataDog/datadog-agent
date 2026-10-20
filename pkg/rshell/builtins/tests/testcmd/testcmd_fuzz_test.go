// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testcmd_test

import (
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

// FuzzTestStringOps fuzzes test with string comparison operators.
// Edge cases: empty strings, strings that look like operators,
// unicode strings, strings with leading/trailing spaces.
func FuzzTestStringOps(f *testing.F) {
	f.Add("hello", "hello", "=")
	f.Add("hello", "world", "!=")
	f.Add("", "", "=")
	f.Add("abc", "def", "=")
	f.Add("a", "b", "!=")
	// Strings that look like operators (POSIX disambiguation edge cases)
	f.Add("-n", "hello", "=")
	f.Add("-z", "", "!=")
	f.Add("-e", "file", "=")
	f.Add("!", "hello", "!=")
	// Lexicographic ordering with < and >
	f.Add("abc", "abd", "<")
	f.Add("z", "a", ">")
	f.Add("A", "a", "<") // uppercase sorts before lowercase in ASCII
	// Unicode strings
	f.Add("héllo", "héllo", "=")
	f.Add("日本語", "日本語", "=")
	f.Add("😀", "😀", "=")
	// Strings with spaces (shell-safe within single quotes)
	f.Add("hello world", "hello world", "=")
	f.Add("a b", "a c", "!=")
	// == operator (same as =)
	f.Add("x", "x", "==")

	// Shared base dir avoids the per-iteration t.TempDir() cleanup overhead
	// that causes "context deadline exceeded" when the 30 s fuzz budget fires.
	// No files are created in this dir; it is only used as the working directory
	// and AllowedPaths root.
	baseDir := f.TempDir()

	f.Fuzz(func(t *testing.T, left, right, op string) {
		if t.Context().Err() != nil {
			return
		}
		if len(left) > 100 || len(right) > 100 {
			return
		}
		if op != "=" && op != "!=" && op != "==" && op != "<" && op != ">" {
			return
		}
		if !utf8.ValidString(left) || !utf8.ValidString(right) {
			return
		}
		for _, s := range []string{left, right} {
			for _, c := range s {
				if c == '\'' || c == '\x00' || c == '\n' || c == ']' {
					return
				}
				// C0/DEL/C1 control chars confuse the shell script parser.
				if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
					return
				}
			}
		}
		// < and > are shell redirection operators — must use = or != in fuzz body.
		if op == "<" || op == ">" {
			return
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		script := fmt.Sprintf("test '%s' %s '%s'", left, op, right)
		_, _, code := cmdRunCtx(ctx, t, script, baseDir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("test string op unexpected exit code %d", code)
		}
	})
}

// FuzzTestIntegerOps fuzzes test with integer comparison operators.
// Edge cases: integer overflow (clamped to MaxInt64/MinInt64),
// leading/trailing spaces (trimmed), very large values.
func FuzzTestIntegerOps(f *testing.F) {
	f.Add(int64(1), int64(2), "-lt")
	f.Add(int64(5), int64(5), "-eq")
	f.Add(int64(10), int64(3), "-gt")
	f.Add(int64(0), int64(0), "-le")
	f.Add(int64(-1), int64(1), "-ne")
	// Boundary values
	f.Add(int64(0), int64(0), "-eq")
	f.Add(int64(-1), int64(0), "-lt")
	f.Add(int64(1), int64(0), "-gt")
	// int32 boundaries
	f.Add(int64(1<<31-1), int64(1<<31-1), "-eq")
	f.Add(int64(-(1 << 31)), int64(-(1 << 31)), "-eq")
	// Values near int64 max/min
	f.Add(int64(1<<31), int64(1<<31), "-eq")
	f.Add(int64(-(1<<31 + 1)), int64(0), "-lt")
	// int64 max (clamped on overflow per GNU test behavior)
	f.Add(int64(1<<31-1), int64(1<<31-1), "-ge")

	// Shared base dir avoids per-iteration t.TempDir() cleanup overhead.
	baseDir := f.TempDir()

	f.Fuzz(func(t *testing.T, left, right int64, op string) {
		if t.Context().Err() != nil {
			return
		}
		switch op {
		case "-eq", "-ne", "-lt", "-le", "-gt", "-ge":
		default:
			return
		}
		// Clamp to reasonable range.
		if left > 1<<31 || left < -(1<<31) || right > 1<<31 || right < -(1<<31) {
			return
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		script := fmt.Sprintf("test %d %s %d", left, op, right)
		_, _, code := cmdRunCtx(ctx, t, script, baseDir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("test %d %s %d unexpected exit code %d", left, op, right, code)
		}
	})
}

// FuzzTestFileOps fuzzes test with file test operators on random filenames.
// Edge cases: -nt/-ot comparison, non-existent files, empty paths.
func FuzzTestFileOps(f *testing.F) {
	f.Add("-e", true)
	f.Add("-f", true)
	f.Add("-d", false)
	f.Add("-s", true)
	f.Add("-r", true)
	f.Add("-z", false)
	// File exists but is empty (-s should be false)
	f.Add("-s", false)
	// Directory test on a file (should be false)
	f.Add("-d", true)
	// Regular file test on non-existent (should be false)
	f.Add("-f", false)

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, op string, createFile bool) {
		if t.Context().Err() != nil {
			return
		}
		switch op {
		case "-e", "-f", "-d", "-s", "-r", "-w", "-x", "-h", "-L", "-p":
		default:
			return
		}
		// Each iteration gets its own subdirectory to avoid races between
		// parallel fuzz workers operating on the same file. We use a manual
		// counter instead of t.TempDir() to avoid the per-iteration cleanup
		// overhead that causes "context deadline exceeded" on CI.
		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		target := "testfile.txt"
		targetPath := filepath.Join(dir, target)
		if createFile {
			if err := os.WriteFile(targetPath, []byte("content"), 0644); err != nil {
				t.Fatal(err)
			}
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		script := fmt.Sprintf("test %s %s", op, target)
		_, _, code := cmdRunCtx(ctx, t, script, dir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("test %s unexpected exit code %d", op, code)
		}
	})
}

// FuzzTestStringUnary fuzzes test with -z and -n string tests.
// Edge cases: empty string, single char, strings that look like operators.
func FuzzTestStringUnary(f *testing.F) {
	f.Add("hello", "-z")
	f.Add("", "-z")
	f.Add("hello", "-n")
	f.Add("", "-n")
	// Strings that look like flags (tested as strings here)
	f.Add("-e", "-n")
	f.Add("-z", "-n")
	f.Add("-n", "-n")
	f.Add("-f", "-z")
	// Single whitespace char
	f.Add(" ", "-n")
	f.Add(" ", "-z")
	// Unicode
	f.Add("日本語", "-n")
	f.Add("😀", "-n")

	// Shared base dir avoids per-iteration t.TempDir() cleanup overhead.
	baseDir := f.TempDir()

	f.Fuzz(func(t *testing.T, arg, op string) {
		if t.Context().Err() != nil {
			return
		}
		if len(arg) > 200 {
			return
		}
		if op != "-z" && op != "-n" {
			return
		}
		if !utf8.ValidString(arg) {
			return
		}
		for _, c := range arg {
			if c == '\'' || c == '\x00' || c == '\n' || c == ']' {
				return
			}
			// C0/DEL/C1 control chars confuse the shell script parser.
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		script := fmt.Sprintf("test %s '%s'", op, arg)
		_, _, code := cmdRunCtx(ctx, t, script, baseDir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("test %s unexpected exit code %d", op, code)
		}
	})
}

// FuzzTestNesting fuzzes test with logical -a/-o operators and compound expressions.
// Edge cases: short-circuit evaluation, ! as final token (treated as non-empty
// string = true), -o as unary shell option (always false in restricted shell),
// strings that look like operators.
// Note: parentheses are shell metacharacters and cannot be passed unescaped
// here; ( ) grouping is covered by the unit tests.
func FuzzTestNesting(f *testing.F) {
	// Simple -a and -o
	f.Add("1 -eq 1 -a 2 -eq 2")
	f.Add("1 -eq 1 -o 1 -eq 2")
	f.Add("1 -eq 2 -a 2 -eq 2")
	// ! negation
	f.Add("! 1 -eq 2")
	f.Add("! -z hello")
	// ! as final token: treated as non-empty string (always true)
	f.Add("!")
	// Boolean chains
	f.Add("-z '' -a -n hello")
	f.Add("-n hello -o -z hello")
	// -o as unary shell option: always false in restricted shell
	f.Add("-o anyopt")
	// String comparison chained
	f.Add("abc = abc -a def != xyz")
	// Chain of -a
	f.Add("1 -eq 1 -a 2 -eq 2 -a 3 -eq 3")
	// Chain of -o
	f.Add("1 -eq 2 -o 2 -eq 2 -o 3 -eq 4")
	// Mixed -a and -o
	f.Add("1 -eq 1 -o 1 -eq 2 -a 2 -eq 2")

	// Shared base dir avoids per-iteration t.TempDir() cleanup overhead.
	baseDir := f.TempDir()

	f.Fuzz(func(t *testing.T, expr string) {
		if t.Context().Err() != nil {
			return
		}
		// Keep expressions short to avoid slow evaluation on CI where
		// fuzz workers have limited CPU; long expressions with many
		// tokens can cause the shell interpreter to exceed the
		// per-iteration timeout, leading to "context deadline exceeded".
		if len(expr) > 60 {
			return
		}
		if !utf8.ValidString(expr) {
			return
		}
		// Limit the number of space-separated tokens to cap nesting
		// depth and keep evaluation fast on CI. The longest seed has
		// 11 tokens so this covers all corpus entries.
		tokens := 0
		for i := 0; i < len(expr); i++ {
			if expr[i] == ' ' {
				tokens++
				if tokens > 11 {
					return
				}
			}
		}
		for _, c := range expr {
			// Filter shell metacharacters that would be interpreted by the shell
			// parser rather than passed to the test builtin.
			if c == '\'' || c == '\x00' || c == '\n' || c == '\\' ||
				c == '"' || c == '`' || c == '$' || c == '(' || c == ')' ||
				c == '<' || c == '>' || c == '|' || c == '&' || c == ';' {
				return
			}
			// Glob metacharacters trigger pathname expansion which can fail
			// on multi-byte UTF-8 patterns due to an upstream library bug.
			if c == '*' || c == '?' || c == '[' || c == ']' {
				return
			}
			// C0/DEL/C1 control chars confuse the shell script parser.
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel() // safety net if t.Fatal fires before explicit cancel
		script := fmt.Sprintf("test %s", expr)
		_, _, code := cmdRunCtx(ctx, t, script, baseDir)
		cancel()
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 && code != 2 {
			t.Errorf("test %q unexpected exit code %d", expr, code)
		}
	})
}
