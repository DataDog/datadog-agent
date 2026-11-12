// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rm_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// FuzzRmFilename fuzzes "rm FILENAME" with arbitrary single-file operands,
// including path-traversal sequences, symlink-like strings, and control
// characters. The fuzzer verifies: no panic, no hang, and exit code is
// always 0 or 1 — never anything else, and rm never removes anything outside
// its own sandboxed temp dir.
func FuzzRmFilename(f *testing.F) {
	f.Add("plain.txt")
	f.Add("")
	f.Add(".")
	f.Add("..")
	f.Add("../secret.txt")
	f.Add("../../../../etc/passwd")
	f.Add("a/b/c")
	f.Add("a//b")
	f.Add("./a")
	f.Add("-")
	f.Add("--")
	f.Add("-oddname")
	f.Add("--verbose")
	f.Add("loop")
	f.Add("a\x00b")
	f.Add("a\nb")
	f.Add(string([]byte{0xff, 0xfe}))
	f.Add("very/deep/nested/path/that/does/not/exist")
	f.Add("~")
	f.Add("*")
	f.Add("?")

	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 256 {
			return
		}
		// Reject inputs that aren't valid UTF-8 or would break shell quoting —
		// the shell parser rejects those before rm ever sees them, which would
		// make this a lexer fuzz test rather than an rm fuzz test.
		if !utf8.ValidString(name) {
			return
		}
		if strings.ContainsAny(name, "\x00'") {
			return
		}
		script := fmt.Sprintf("rm '%s'", name)
		// A byte-level filter on name isn't sufficient: e.g. a trailing
		// backslash combines with our appended closing quote in ways the
		// shell lexer's line-continuation handling can still reject as
		// unparseable. Pre-parse the actual constructed script and drop
		// anything the parser itself rejects, mirroring du's fuzz test.
		if _, err := syntax.NewParser().Parse(strings.NewReader(script), ""); err != nil {
			return
		}

		dir := t.TempDir()
		outside := t.TempDir()
		secret := filepath.Join(outside, "secret.txt")
		if err := os.WriteFile(secret, []byte("secret"), 0644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, _, code := runScriptCtx(ctx, t, script, dir,
			interp.AllowedPaths([]string{dir + ":rw"}),
			interp.WithMode(interp.ModeRemediation),
		)
		if ctx.Err() != nil {
			t.Fatal("rm fuzz timed out")
		}
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d for filename %q", code, name)
		}
		if _, err := os.Stat(secret); err != nil {
			t.Errorf("rm %q must never affect files outside its sandbox root: %v", name, err)
		}
	})
}

// FuzzRmOperandCount fuzzes the number of file operands passed to rm,
// verifying the MaxRemoveFiles cap is enforced consistently: invocations at
// or under the cap may succeed, invocations over the cap must be rejected
// outright with none of the files removed (no partial deletion on a
// cap-exceeded invocation).
func FuzzRmOperandCount(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(9)
	f.Add(10)
	f.Add(11)
	f.Add(50)
	f.Add(200)

	f.Fuzz(func(t *testing.T, count int) {
		if count < 0 || count > 500 {
			return
		}

		dir := t.TempDir()
		names := make([]string, 0, count)
		for i := range count {
			name := fmt.Sprintf("f%d.txt", i)
			names = append(names, name)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
				t.Fatal(err)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		script := "rm " + join(names)
		if count == 0 {
			script = "rm"
		}
		_, _, code := runScriptCtx(ctx, t, script, dir,
			interp.AllowedPaths([]string{dir + ":rw"}),
			interp.WithMode(interp.ModeRemediation),
		)
		if ctx.Err() != nil {
			t.Fatal("rm fuzz timed out")
		}

		if count > 10 {
			if code != 1 {
				t.Errorf("expected exit 1 for %d operands (cap exceeded), got %d", count, code)
			}
			for _, n := range names {
				if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
					t.Errorf("cap-exceeded invocation must not remove any file, but %q is gone", n)
				}
			}
			return
		}
		if count == 0 {
			if code != 1 {
				t.Errorf("expected exit 1 for missing operand, got %d", code)
			}
			return
		}
		if code != 0 {
			t.Errorf("expected exit 0 for %d operands (within cap), got %d", count, code)
		}
		for _, n := range names {
			if _, err := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(err) {
				t.Errorf("expected %q to be removed", n)
			}
		}
	})
}
