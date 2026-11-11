// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cd_test

import (
	"context"
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

// cdRunCtxFuzz is the fuzz-friendly script runner. It uses a
// per-iteration subdirectory under base, with AllowedPaths scoped to
// that subdirectory.
func cdRunCtxFuzz(ctx context.Context, t *testing.T, script, base string) (string, string, int) {
	t.Helper()
	dir, cleanup := testutil.FuzzIterDir(t, base, &fuzzCounter)
	defer cleanup()
	return testutil.RunScriptCtx(ctx, t, script, dir, interp.AllowedPaths([]string{dir}))
}

// shellSafe rejects bytes that would change shell tokenization or
// expansion in a way the fuzzer cannot recover from. Same policy as
// pwd_fuzz_test.go — the fuzzer cares about cd's behavior, not the
// shell parser's, so we filter aggressively.
func shellSafe(s string) bool {
	if len(s) > 1024 {
		return false
	}
	if !utf8.ValidString(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7F {
			return false
		}
		switch s[i] {
		case '`', '$', '\\', '"', '\'', '|', '&', ';', '<', '>', '(', ')':
			return false
		case '*', '?', '[', ']', '{', '}':
			return false
		case '~':
			return false
		case '#':
			return false
		}
	}
	return true
}

// validExit lists the codes cd / the interpreter sandbox can
// legitimately produce. 0 = success, 1 = cd error, 2 = sandbox-level
// rejection (e.g. unsupported syntax). Anything else indicates a defect.
func validExit(code int) bool {
	return code == 0 || code == 1 || code == 2
}

// FuzzCdFlags fuzzes flag-shape arguments to cd. The cd builtin must
// always exit 0 or 1 — never panic, never hang, never produce another
// code.
//
// Seed corpus draws from three sources (per the implement-posix-command
// skill):
//
//	A. Implementation edge cases (every recognized flag, hop cap, ".",
//	   "..", boundary forms).
//	B. CVE-class adversarial shapes (long inputs, embedded nulls — though
//	   nulls are rejected by shellSafe — bogus flag forms).
//	C. Inputs from the black-box test file so regressions stay covered
//	   by the fuzz baseline.
func FuzzCdFlags(f *testing.F) {
	// Source A: every flag and obvious adversarial shape.
	for _, seed := range []string{
		"", // no args
		"-L", "-P", "-h",
		"--logical", "--physical", "--help",
		"-LP", "-PL", "-Lh",
		"--", "-", "---",
		"--LL",
		"-x",                     // unknown short
		"--no-flag",              // unknown long
		"--version",              // GNU-but-not-supported
		"--logical=",             // empty value
		"--physical=true",        // explicit value rejected
		"--logical=false",        // explicit value rejected
		strings.Repeat("a", 256), // long positional
		"a b c",                  // too many args
	} {
		f.Add(seed)
	}

	// Source B: combinations and CVE-class shapes.
	for _, seed := range []string{
		"-L -P", "-P -L", "-L --physical", "--logical -P",
		"-h .", "--help foo",
		"-- -P",
		"-- -",
		"--logical=true",
		strings.Repeat("-L ", 50), // many flag tokens
	} {
		f.Add(seed)
	}

	// Source C: shapes from the black-box test file.
	for _, seed := range []string{
		"-L -P x",
		"-- x",
		"-h",
		"-x",
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
		_, _, code := cdRunCtxFuzz(ctx, t, "cd "+args, base)
		if !validExit(code) {
			t.Errorf("unexpected exit code %d for args %q", code, args)
		}
	})
}

// FuzzCdPath fuzzes path operands to cd. The directory does not need
// to exist; cd is expected to fail gracefully when it does not.
func FuzzCdPath(f *testing.F) {
	// Source A: implementation edge cases for paths.
	for _, seed := range []string{
		".",
		"..",
		"./",
		"../",
		"./.",
		"./..",
		"../..",
		"//",                      // double slash
		"///",                     // triple
		"./a",                     // relative with .
		"../a",                    // relative with ..
		"a/b/c",                   // nested relative
		"a//b",                    // double sep mid-path
		"a/./b",                   // dot mid-path
		"a/../b",                  // dotdot mid-path
		"-",                       // OLDPWD form
		"x",                       // simple relative
		"x ",                      // trailing space
		" x",                      // leading space
		strings.Repeat("a/", 100), // many components
	} {
		f.Add(seed)
	}

	// Source B: CVE-class. Long paths, deep dotdot escapes, control-y
	// strings (filtered by shellSafe), reserved-ish names.
	for _, seed := range []string{
		strings.Repeat("../", 50), // long dotdot chain
		strings.Repeat(".", 1000), // many dots
		"CON",                     // Windows reserved name
		"NUL",
		"PRN",
		"AUX",
	} {
		f.Add(seed)
	}

	// Source C: inputs that appear in scenario tests.
	for _, seed := range []string{
		"a/b",
		"a/b/c",
		"deep/x/y",
		"child",
	} {
		f.Add(seed)
	}

	base := f.TempDir()
	f.Fuzz(func(t *testing.T, path string) {
		if !shellSafe(path) {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), fuzzTimeout)
		defer cancel()
		_, _, code := cdRunCtxFuzz(ctx, t, "cd "+path, base)
		if !validExit(code) {
			t.Errorf("unexpected exit code %d for path %q", code, path)
		}
	})
}
