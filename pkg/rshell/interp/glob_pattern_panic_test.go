// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBraceGeneratedInvalidGlobDoesNotPanic is a regression test for a crash
// found by fuzzing the test builtin (FuzzTestNesting). Brace expansion can
// synthesize a glob pattern that mvdan.cc/sh's matcher cannot compile — for
// example `{a..A}` expands over the ASCII range that includes '[' and ']', so
// `test {a..A}1-{a..A}0{a..A}` produces a field like `[1-0]`, an invalid
// character-class range. mvdan's ExtendedPatternMatcher calls regexp.MustCompile
// and panics on it; before the fix that panic reached Run's recovery and
// surfaced as the generic "internal error" sentinel (a non-ExitStatus error).
// Field expansion now converts it into a clean expansion failure.
func TestBraceGeneratedInvalidGlobDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	scripts := map[string]string{
		"brace_sequence_forms_invalid_class": "test {a..A}1-{a..A}0{a..A}",
		"brace_builds_invalid_class":         "echo {a..A}1-{a..A}0",
		"direct_invalid_class":               "echo [1-0]",
		"reversed_class":                     "echo [z-a]",
	}
	for name, script := range scripts {
		t.Run(name, func(t *testing.T) {
			var out, errb bytes.Buffer
			r, err := New(StdIO(nil, &out, &errb), allowAllCommandsOpt(), AllowedPaths([]string{dir}))
			require.NoError(t, err)
			defer r.Close()
			r.Dir = dir

			prog, err := ParseScript(script, "")
			require.NoError(t, err)

			rerr := r.Run(context.Background(), prog)
			// The run must terminate with a normal exit status, never the
			// recovered-panic sentinel or any other non-ExitStatus error.
			if rerr != nil {
				var es ExitStatus
				if !errors.As(rerr, &es) {
					t.Fatalf("expected a clean exit, got non-exit error %q (stderr=%q)", rerr, errb.String())
				}
			}
			if strings.Contains(errb.String(), "internal panic") {
				t.Fatalf("interpreter panicked instead of handling the pattern: %s", errb.String())
			}
		})
	}
}

// TestValidGlobStillExpands guards against the invalid-pattern fix regressing
// ordinary, valid globbing.
func TestValidGlobStillExpands(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		script string
		want   string
	}{
		{"echo *.txt", "a.txt b.txt\n"},
		{"echo [ab].txt", "a.txt b.txt\n"},
		{"echo [a-z].txt", "a.txt b.txt\n"},
		{"echo nomatch*.zzz", "nomatch*.zzz\n"}, // no match: word stays literal
	} {
		var out, errb bytes.Buffer
		r, err := New(StdIO(nil, &out, &errb), allowAllCommandsOpt(), AllowedPaths([]string{dir}))
		require.NoError(t, err)
		r.Dir = dir
		prog, err := ParseScript(tc.script, "")
		require.NoError(t, err)
		if err := r.Run(context.Background(), prog); err != nil {
			var es ExitStatus
			if !errors.As(err, &es) {
				r.Close()
				t.Fatalf("%q: unexpected error: %v", tc.script, err)
			}
		}
		r.Close()
		if out.String() != tc.want {
			t.Errorf("%q: got %q, want %q", tc.script, out.String(), tc.want)
		}
	}
}
