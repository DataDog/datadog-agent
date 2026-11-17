// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package find

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExecDirParentDir verifies the trim-and-Dir logic used to compute the
// parent directory for -execdir. Trailing slashes are stripped (except root
// and Windows drive roots) before filepath.Dir is called.
func TestExecDirParentDir(t *testing.T) {
	tests := []struct {
		name    string
		absPath string
		want    string
	}{
		{"trailing slash trimmed", "/cwd/dir/", "/cwd"},
		{"no trailing slash", "/cwd/dir", "/cwd"},
		{"dot preserved", "/cwd/.", "/cwd"},
		{"unix root", "/", "/"},
		{"colon in unix path", "/tmp/d:/", "/tmp"},
		{"deeply nested trailing", "/a/b/c/", "/a/b"},
		{"multiple trailing slashes", "/a/b//", "/a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filepath.ToSlash(execDirParentDir(tt.absPath))
			assert.Equal(t, tt.want, got, "execDirParentDir(%q)", tt.absPath)
		})
	}
}

// TestIsExpressionStart verifies the boundary between path operands and
// expression tokens. GNU find treats !, (, and any dash-prefixed token
// with length > 1 as expression starters. Everything else (including
// ")", "-", and plain words) is a path operand.
func TestIsExpressionStart(t *testing.T) {
	tests := []struct {
		arg  string
		want bool
	}{
		// Expression starters
		{"!", true},
		{"(", true},
		{"-name", true},
		{"-type", true},
		{"-maxdepth", true},
		{"-1", true}, // unknown predicate, but still expression
		{"-a", true}, // short flag-like token
		{"--", true}, // double dash, length > 1 and starts with -

		// Path operands (NOT expression starters)
		{")", false},       // closing paren is a path, not expression
		{"-", false},       // single dash is a path (length 1)
		{".", false},       // current dir
		{"..", false},      // parent dir
		{"foo", false},     // plain word
		{"/tmp", false},    // absolute path
		{"dir/sub", false}, // relative path
		{"", false},        // empty string
	}

	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			got := isExpressionStart(tt.arg)
			assert.Equal(t, tt.want, got, "isExpressionStart(%q)", tt.arg)
		})
	}
}
