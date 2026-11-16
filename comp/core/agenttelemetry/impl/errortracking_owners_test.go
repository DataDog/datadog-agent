// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnerOfPath(t *testing.T) {
	const teams = "alpha\nbeta\ngamma"
	// Rows are in file order, so later rows win.
	const rules = `0 *
1 /pkg/logs/
2 /pkg/logs/sender/
- /pkg/logs/sender/vendored/
0 /docs/*
1 build/
2 /cmd/**/*_windows*.go
1 /comp/**/impl
0 /exact.go
2 /gen?.go
1 **/gen.go`

	tests := []struct {
		name string
		path string
		want string
	}{
		{"default row owns everything else", "other/file.go", "alpha"},
		{"anchored directory", "pkg/logs/x.go", "beta"},
		{"anchored directory is not a suffix match", "vendor/pkg/logs/x.go", "alpha"},
		{"last match wins over an earlier broader row", "pkg/logs/sender/x.go", "gamma"},
		{"last match can name no team", "pkg/logs/sender/vendored/x.go", ""},
		{"trailing slash does not match a file of that name", "pkg/logs", "alpha"},
		{"single star stays in one segment", "docs/a.md", "alpha"},
		{"single star names a directory that owns its contents", "docs/a/b.md", "alpha"},
		{"unanchored directory matches at any depth", "x/y/build/z.go", "beta"},
		{"unanchored directory needs contents", "x/build", "alpha"},
		{"double star spans segments", "cmd/agent/sub/main_windows.go", "gamma"},
		{"double star spans no segment", "cmd/main_windows.go", "gamma"},
		{"star inside a file name", "cmd/a/b_windows_amd64.go", "gamma"},
		{"star pattern keeps the extension", "cmd/a/b_windows.txt", "alpha"},
		{"double star in the middle with a name", "comp/core/log/impl/x.go", "beta"},
		{"exact file", "exact.go", "alpha"},
		{"exact file anchored: not nested", "sub/exact.go", "alpha"},
		{"question mark matches one character", "gen1.go", "gamma"},
		{"question mark does not match none", "gen.go", "beta"},
		{"leading double star matches the root", "gen.go", "beta"},
		{"leading double star matches nested", "a/b/gen.go", "beta"},
	}
	compiled := parseOwnerRules(teams, rules)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ownerOfPath(compiled, tt.path))
		})
	}
}

func TestOwnerOfPathNoDefaultRow(t *testing.T) {
	rules := parseOwnerRules("alpha", "0 /pkg/")
	assert.Equal(t, "", ownerOfPath(rules, "cmd/main.go"))
}

func TestParseOwnerRulesDropsBadRows(t *testing.T) {
	rules := parseOwnerRules("alpha", "0 /a/\nnospace\nx /b/\n9 /c/\n- /d/")
	require.Len(t, rules, 2)
	assert.Equal(t, "alpha", ownerOfPath(rules, "a/f.go"))
	assert.Equal(t, "", ownerOfPath(rules, "d/f.go"))
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"*", "", true},
		{"*", "anything", true},
		{"a*c", "abc", true},
		{"a*c", "ac", true},
		{"a*c", "abd", false},
		{"*_windows*.go", "x_windows_amd64.go", true},
		{"*_windows*.go", "x_linux.go", false},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"**", "abc", true},
		{"exact", "exact", true},
		{"exact", "exactly", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, matchGlob(tt.pattern, tt.name), "%q vs %q", tt.pattern, tt.name)
	}
}

// TestGeneratedTableMatchesPythonParser replays paths whose owners were
// resolved by the Python CODEOWNERS parser in tasks/libs/owners. The file is
// written by `dda inv errortracking-owners.generate`: one synthesized path per
// CODEOWNERS row plus real sample paths.
func TestGeneratedTableMatchesPythonParser(t *testing.T) {
	data, err := os.ReadFile("testdata/errortracking_owners_parity.tsv")
	require.NoError(t, err)
	rules := codeOwnersRules()
	require.NotEmpty(t, rules)

	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	require.Greater(t, len(lines), 100)
	for _, line := range lines {
		path, want, _ := strings.Cut(line, "\t")
		assert.Equal(t, want, ownerOfPath(rules, path), path)
	}
}

func BenchmarkOwnerOfPath(b *testing.B) {
	rules := codeOwnersRules()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ownerOfPath(rules, "pkg/logs/launchers/file/launcher.go")
	}
}

// TestGeneratedTableZeroDirectories pins the "**" zero-directory case where the
// Go matcher follows GitHub and the Python parser differs.
func TestGeneratedTableZeroDirectories(t *testing.T) {
	rules := codeOwnersRules()
	assert.Equal(t, "fleet", ownerOfPath(rules, "cmd/installer/x_windows.go"))
	assert.Equal(t, "fleet", ownerOfPath(rules, "cmd/installer/sub/x_windows.go"))
}
