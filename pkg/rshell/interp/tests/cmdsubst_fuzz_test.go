// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tests_test

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"
)

// FuzzCmdSubstEcho fuzzes command substitution with echo and various arguments.
func FuzzCmdSubstEcho(f *testing.F) {
	// Seed from existing test cases
	f.Add("hello")
	f.Add("")
	f.Add("hello world")
	f.Add("hello   world")
	f.Add("a\tb\tc")
	f.Add("line1\nline2")
	f.Add("trailing\n\n\n")
	f.Add("héllo wörld")
	f.Add("日本語")

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
		// Filter out characters that break shell parsing
		for _, c := range arg {
			if c == '\'' || c == '\x00' || c == '`' || c == '$' || c == '\\' {
				return
			}
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}

		dir := t.TempDir()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, _, code := cmdSubstRunCtx(ctx, t, `x=$(echo '`+arg+`'); echo "$x"`, dir)
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d for arg %q", code, arg)
		}
	})
}

// FuzzCmdSubstNested fuzzes nested command substitution.
func FuzzCmdSubstNested(f *testing.F) {
	f.Add("hello")
	f.Add("a b c")
	f.Add("")

	f.Fuzz(func(t *testing.T, arg string) {
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
			if c == '\'' || c == '\x00' || c == '`' || c == '$' || c == '\\' || c == ')' || c == '(' {
				return
			}
			// Glob metacharacters trigger pathname expansion in the
			// unquoted $(…) result, which can hit an upstream UTF-8
			// bug in the pattern library. Filter them out since this
			// fuzz target tests command substitution, not globbing.
			if c == '*' || c == '?' || c == '[' {
				return
			}
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}

		dir := t.TempDir()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, _, code := cmdSubstRunCtx(ctx, t, `echo $(echo '`+arg+`')`, dir)
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d for arg %q", code, arg)
		}
	})
}

// FuzzSubshellCommands fuzzes subshell with various safe commands.
func FuzzSubshellCommands(f *testing.F) {
	f.Add("hello")
	f.Add("a b c")
	f.Add("")
	f.Add("hello world")

	f.Fuzz(func(t *testing.T, arg string) {
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
			if c == '\'' || c == '\x00' || c == '`' || c == '$' || c == '\\' || c == ')' || c == '(' {
				return
			}
			if c < 0x20 || c == 0x7f || (c >= 0x80 && c < 0xa0) {
				return
			}
		}

		dir := t.TempDir()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, _, code := subshellRunCtx(ctx, t, `(echo '`+arg+`')`, dir)
		if t.Context().Err() != nil {
			return
		}
		if code != 0 && code != 1 {
			t.Errorf("unexpected exit code %d for arg %q", code, arg)
		}
	})
}
