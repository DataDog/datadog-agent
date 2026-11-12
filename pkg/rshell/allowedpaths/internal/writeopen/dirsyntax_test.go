// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// These are Go tests by necessity: writeopen is an internal package and the
// YAML scenario framework cannot reach it directly, so AGENTS.md's "prefer
// scenario tests" rule does not apply here.

package writeopen

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestHasTrailingDirSyntax(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		want    bool
	}{
		{name: "empty", relPath: "", want: false},
		{name: "plain file", relPath: "f", want: false},
		{name: "trailing slash", relPath: "f/", want: true},
		{name: "dot", relPath: ".", want: true},
		{name: "dotdot", relPath: "..", want: true},
		{name: "nested dot", relPath: "a/.", want: true},
		{name: "nested dotdot", relPath: "a/..", want: true},
		{name: "nested file", relPath: "a/b", want: false},
		{name: "nested trailing slash", relPath: "a/b/", want: true},
		{name: "root", relPath: "/", want: true},
		{name: "dotfile is not dot syntax", relPath: "a/.hidden", want: false},
		{name: "dotdot prefixed name", relPath: "a/..b", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasTrailingDirSyntax(tc.relPath); got != tc.want {
				t.Errorf("HasTrailingDirSyntax(%q) = %v, want %v", tc.relPath, got, tc.want)
			}
		})
	}
}

// TestHasTrailingDirSyntaxBackslash pins the platform-conditional backslash
// branch. filepath.Separator is a compile-time constant per platform, so the
// expectation has to be selected by runtime.GOOS: on Windows a trailing "\"
// is directory syntax, on Unix "\" is an ordinary filename character.
func TestHasTrailingDirSyntaxBackslash(t *testing.T) {
	wantTrailing := runtime.GOOS == "windows"
	if got := HasTrailingDirSyntax(`f\`); got != wantTrailing {
		t.Errorf(`HasTrailingDirSyntax("f\\") = %v, want %v (separator %q)`, got, wantTrailing, filepath.Separator)
	}
	// A backslash in the middle is never trailing directory syntax; on
	// Windows it is a separator so Base is "b", on Unix the whole thing is
	// one filename. Either way: false.
	if got := HasTrailingDirSyntax(`a\b`); got {
		t.Errorf(`HasTrailingDirSyntax("a\\b") = true, want false`)
	}
}
