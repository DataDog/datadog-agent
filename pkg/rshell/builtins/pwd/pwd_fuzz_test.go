// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pwd_test

import (
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

// fuzzCounter generates unique per-iteration subdir names.
var fuzzCounter atomic.Int64

const fuzzTimeout = 5 * time.Second

// pwdRunCtxFuzz is the fuzz-friendly script runner. It uses a per-iteration
// subdirectory under base, with AllowedPaths scoped to that subdirectory.
func pwdRunCtxFuzz(ctx context.Context, t *testing.T, script, base string) (string, string, int) {
	t.Helper()
	dir, cleanup := testutil.FuzzIterDir(t, base, &fuzzCounter)
	defer cleanup()
	return testutil.RunScriptCtx(ctx, t, script, dir, interp.AllowedPaths([]string{dir}))
}

// shellSafe rejects bytes that would change shell tokenization or
// expansion in a way the fuzzer cannot recover from. Parse errors,
// glob-regex compile failures, and tilde expansion all surface as
// non-ExitStatus errors that fail the test before pwd is even invoked.
// The fuzzer cares about pwd's behavior, not the shell parser's, so we
// filter aggressively. Inputs are also capped at 1 KiB and required to
// be valid UTF-8 (the parser rejects invalid encodings).
func shellSafe(s string) bool {
	if len(s) > 1024 {
		return false
	}
	if !utf8.ValidString(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		// Reject all ASCII control characters (0x00–0x1F and 0x7F).
		// Shell tokenization differs from Go's strings.Fields on
		// characters like \v and \f: the shell treats them as part of
		// a token while Fields splits on them, which confuses
		// flag-token-detection in this test.
		if s[i] < 0x20 || s[i] == 0x7F {
			return false
		}
		switch s[i] {
		// Control / quoting / redirection / substitution.
		case '`', '$', '\\', '"', '\'', '|', '&', ';', '<', '>', '(', ')':
			return false
		// Glob/brace/range expansion — these reach mvdan/sh's expander
		// which can hit upstream bugs on weird inputs (e.g. "怎*"
		// triggers a regex-compile failure on an invalid-UTF-8 prefix).
		case '*', '?', '[', ']', '{', '}':
			return false
		// Tilde expansion is unsupported by the runner and rejected at
		// the interpreter layer with a fixed exit code 2 — already
		// covered by `validExit`, but excluding it keeps the corpus
		// focused on real pwd-arg shapes.
		case '~':
			return false
		// Comments — the rest of the line after `#` is dropped by the
		// shell parser, so a `-h` after `#` never reaches pwd.
		case '#':
			return false
		}
	}
	return true
}

// FuzzPwdArgs fuzzes the pwd command's argument parser. The pwd builtin
// accepts -L, -P, -h, --logical, --physical, --help, and ignores any
// positional arguments. The fuzzer tries arbitrary single-token args:
// the command must always exit cleanly (0 for success, 1 for invalid
// flag) — never panic, never block, never produce a different code.
func FuzzPwdArgs(f *testing.F) {
	// Implementation edge cases — cover every flag and obvious adversarial
	// shapes for pflag.
	for _, seed := range []string{
		"",   // no args
		"-L", // logical short
		"-P", // physical short
		"-h", // help short
		"--logical", "--physical", "--help",
		"-LP",                    // combined short flags
		"-PL",                    // combined short flags, swapped
		"-Lh",                    // logical + help
		"--",                     // end-of-flags only
		"-",                      // bare dash
		"---",                    // triple dash
		"--LL",                   // bogus long
		"-x",                     // unknown short
		"--no-flag",              // unknown long
		"--version",              // GNU-but-not-supported flag
		"--logical=",             // long with empty value (boolean rejects)
		"x",                      // bare positional
		"-",                      // bare dash
		"hello world",            // spaces
		strings.Repeat("a", 256), // long arg
	} {
		f.Add(seed)
	}

	// Existing-test inputs: every concrete invocation pattern from the
	// black-box test file should be in the corpus baseline so a regression
	// stays caught by fuzz coverage.
	for _, seed := range []string{
		"-L -P", "-P -L", "-L --physical", "--logical -P",
		"-- foo", "-- --not-a-flag", "extra args",
	} {
		f.Add(seed)
	}

	base := f.TempDir()
	f.Fuzz(func(t *testing.T, args string) {
		if !shellSafe(args) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), fuzzTimeout)
		defer cancel()
		_, _, code := pwdRunCtxFuzz(ctx, t, "pwd "+args, base)
		// pwd itself produces 0 (success) or 1 (flag error). The shell
		// runner can return 2 for sandbox rejections (e.g. tilde
		// expansion) before pwd ever runs — that's an interpreter
		// concern, not pwd's. Anything else indicates a defect.
		if !validExit(code) {
			t.Errorf("unexpected exit code %d for args %q", code, args)
		}
	})
}

// validExit accepts the exit codes pwd or the interpreter's sandbox
// layer can legitimately produce. 130 is SIGINT-like (context cancel
// during expansion), 137 is SIGKILL — neither should escape, but they
// indicate runtime termination, not a defect in pwd.
func validExit(code int) bool {
	return code == 0 || code == 1 || code == 2
}

// FuzzPwdFlagsCombo fuzzes combinations of recognized flags and
// positional arguments. The fuzzer feeds arbitrary tokens to pwd and
// verifies that pwd never panics, never hangs, and always returns a
// known exit code.
func FuzzPwdFlagsCombo(f *testing.F) {
	for _, seed := range []string{
		"-L -P", "-P -L", "-L --physical", "--logical -P",
		"-LP", "-PL", "-Lh",
		"-h foo bar", "--help --no-such",
		"-- foo", "-- --logical",
		"--help -P", "-P --help",
		"-L=true", "-P=false",
		"--logical=true", "--physical=true",
	} {
		f.Add(seed)
	}

	base := f.TempDir()
	f.Fuzz(func(t *testing.T, args string) {
		if !shellSafe(args) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), fuzzTimeout)
		defer cancel()
		_, _, code := pwdRunCtxFuzz(ctx, t, "pwd "+args, base)
		if !validExit(code) {
			t.Errorf("unexpected exit code %d for args %q", code, args)
		}
	})
}

// FuzzPwdSymlinkTargets fuzzes -P resolution against symlink chains
// whose targets are arbitrary user-provided strings. The resolver must
// terminate (within the hop cap) for any input — never panic, never
// hang.
func FuzzPwdSymlinkTargets(f *testing.F) {
	// Implementation edge cases + CVE-style adversarial inputs + existing
	// test inputs all share the same fuzz target shape (single string).
	seeds := []string{
		// Implementation edge cases.
		"target",                   // simple relative
		"./target",                 // dot prefix
		"../target",                // dot-dot prefix
		"../../target",             // multiple dot-dots
		"/abs/target",              // absolute target
		"/",                        // root
		"./.",                      // dot only
		"sub/lnk",                  // multi-component
		"a/b/c/d/e/f/g/h",          // deep
		string(filepath.Separator), // bare separator
		"",                         // empty (invalid)
		strings.Repeat("a", 200),   // long name
		"target/with spaces",       // spaces
		// CVE / weird-input class.
		"target\x00null",                      // embedded NUL — Symlink rejects
		"target/with/many/slashes",            // benign multi-component
		"a/" + strings.Repeat("b/", 30) + "z", // 60-component path
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	base := f.TempDir()
	f.Fuzz(func(t *testing.T, target string) {
		// Reject obviously-illegal symlink targets up front — Symlink will
		// fail and we'd just skip the iteration anyway.
		if len(target) == 0 || len(target) > 200 || strings.ContainsRune(target, 0) {
			return
		}
		dir, cleanup := testutil.FuzzIterDir(t, base, &fuzzCounter)
		defer cleanup()
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), fuzzTimeout)
		defer cancel()
		_, _, code := testutil.RunScriptCtx(ctx, t, "pwd -P", link, interp.AllowedPaths([]string{dir}))
		if !validExit(code) {
			t.Errorf("unexpected exit code %d for symlink target %q", code, target)
		}
	})
}
