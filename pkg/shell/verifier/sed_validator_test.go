// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package verifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSedScript_Allowed(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{name: "simple substitution", script: `s/foo/bar/`},
		{name: "global substitution", script: `s/foo/bar/g`},
		{name: "substitution with flags", script: `s/foo/bar/gp`},
		{name: "delete command", script: `d`},
		{name: "print command", script: `p`},
		{name: "transliterate", script: `y/abc/ABC/`},
		{name: "address range with delete", script: `1,5d`},
		{name: "regex address", script: `/pattern/d`},
		{name: "dollar address", script: `$d`},
		{name: "branch command", script: `b label`},
		{name: "test command", script: `t label`},
		{name: "append text", script: `a appended text`},
		{name: "insert text", script: `i inserted text`},
		{name: "change text", script: `c changed text`},
		{name: "braces", script: `/pat/{d}`},
		{name: "comment", script: `# this is a comment`},
		{name: "multiple commands", script: "s/a/b/;s/c/d/"},
		{name: "bracket expr in pattern", script: `s/[abc]/x/g`},
		{name: "bracket expr with delimiter inside", script: `s/[/]/x/g`},
		{name: "negated bracket expr", script: `s/[^abc]/x/g`},
		{name: "bracket with leading ]", script: `s/[]abc]/x/g`},
		{name: "bracket with negation and leading ]", script: `s/[^]abc]/x/g`},
		{name: "pipe delimiter with bracket", script: `s|[|]|x|g`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSedScript(tt.script)
			assert.NoError(t, err, "script should be allowed: %s", tt.script)
		})
	}
}

func TestValidateSedScript_Blocked(t *testing.T) {
	tests := []struct {
		name       string
		script     string
		wantSubstr string
	}{
		{name: "e command", script: `e`, wantSubstr: "'e' command"},
		{name: "w command", script: `w /tmp/output`, wantSubstr: "'w'"},
		{name: "W command", script: `W /tmp/output`, wantSubstr: "'w'"},
		{name: "r command", script: `r /etc/passwd`, wantSubstr: "'r'"},
		{name: "R command", script: `R /etc/passwd`, wantSubstr: "'r'"},
		{name: "s///e flag", script: `s/foo/bar/e`, wantSubstr: "'s///e'"},
		{name: "s///w flag", script: `s/foo/bar/w /tmp/out`, wantSubstr: "'s///w'"},
		{name: "s///ge flags", script: `s/foo/bar/ge`, wantSubstr: "'s///e'"},
		{name: "e after semicolon", script: `s/a/b/; e`, wantSubstr: "'e' command"},
		{name: "w after newline", script: "s/a/b/\nw /tmp/out", wantSubstr: "'w'"},
		{name: "e with address", script: `1e`, wantSubstr: "'e' command"},
		{name: "e with regex address", script: `/pattern/e`, wantSubstr: "'e' command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSedScript(tt.script)
			require.Error(t, err, "script should be blocked: %s", tt.script)
			assert.Contains(t, err.Error(), tt.wantSubstr)
		})
	}
}

func TestValidateSedScript_BracketExprDoesNotConfuseDelimiters(t *testing.T) {
	// The delimiter inside [...] must NOT be treated as the closing delimiter.
	// Without bracket-awareness, the scanner would misparse and miss the 'e' flag.
	tests := []struct {
		name   string
		script string
	}{
		{
			name:   "e flag with delimiter in bracket expr",
			script: `s/[/]/replacement/e`,
		},
		{
			name:   "w flag with delimiter in bracket expr",
			script: `s/[/]/replacement/w /tmp/out`,
		},
		{
			name:   "e flag with negated bracket containing delimiter",
			script: `s/[^/]/replacement/e`,
		},
		{
			name:   "e flag with pipe delimiter in bracket",
			script: `s|[|]|replacement|e`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSedScript(tt.script)
			require.Error(t, err, "dangerous flag should be caught even with bracket expr: %s", tt.script)
		})
	}
}

func TestValidateSedArgs_WithFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "safe -e script", args: []string{"-e", "s/a/b/"}, wantErr: false},
		{name: "dangerous -e script", args: []string{"-e", "e"}, wantErr: true},
		{name: "safe combined -e", args: []string{"-es/a/b/"}, wantErr: false},
		{name: "dangerous combined -e", args: []string{"-ee"}, wantErr: true},
		{name: "multiple -e safe", args: []string{"-e", "s/a/b/", "-e", "s/c/d/"}, wantErr: false},
		{name: "multiple -e one dangerous", args: []string{"-e", "s/a/b/", "-e", "e"}, wantErr: true},
		{name: "positional script safe", args: []string{"s/a/b/", "file.txt"}, wantErr: false},
		{name: "positional script dangerous", args: []string{"e", "file.txt"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSedArgs(tt.args)
			if tt.wantErr {
				require.Error(t, err, "should be blocked: %v", tt.args)
			} else {
				require.NoError(t, err, "should be allowed: %v", tt.args)
			}
		})
	}
}
