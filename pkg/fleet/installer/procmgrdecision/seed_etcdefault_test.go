// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgrdecision

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSeedFile writes content to a file in a fresh temporary directory and returns its path.
func writeSeedFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "datadog-agent-procmgr")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestSeedFromEtcDefaultFound(t *testing.T) {
	tests := []struct {
		name    string
		content string
		enabled bool
	}{
		{name: "assignment no", content: "DATADOG_ENABLED=no\n", enabled: false},
		{name: "assignment yes", content: "DATADOG_ENABLED=yes\n", enabled: true},
		{name: "empty value is enabled", content: "DATADOG_ENABLED=\n", enabled: true},
		{name: "double quoted no", content: "DATADOG_ENABLED=\"no\"\n", enabled: false},
		{name: "single quoted no", content: "DATADOG_ENABLED='no'\n", enabled: false},
		{name: "capitalized No is enabled", content: "DATADOG_ENABLED=No\n", enabled: true},
		{name: "export prefix", content: "export DATADOG_ENABLED=no\n", enabled: false},
		{name: "export with tab", content: "export\tDATADOG_ENABLED=no\n", enabled: false},
		{name: "trailing comment", content: "DATADOG_ENABLED=no # off\n", enabled: false},
		{name: "quoted value with trailing comment", content: "DATADOG_ENABLED=\"no\" # off\n", enabled: false},
		{name: "quoted value with trailing spaces", content: "DATADOG_ENABLED=\"no\"   \n", enabled: false},
		{name: "indented assignment outside a block", content: "  DATADOG_ENABLED=no\n", enabled: false},
		{name: "crlf", content: "DATADOG_ENABLED=no\r\n", enabled: false},
		{name: "no trailing newline", content: "DATADOG_ENABLED=no", enabled: false},
		{name: "last assignment wins: no then yes", content: "DATADOG_ENABLED=no\nDATADOG_ENABLED=yes\n", enabled: true},
		{name: "last assignment wins: yes then no", content: "DATADOG_ENABLED=yes\nDATADOG_ENABLED=no\n", enabled: false},
		{name: "other variables are ignored", content: "FOO=no\nBAR=1\nDATADOG_ENABLED=no\n", enabled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeSeedFile(t, tt.content)

			res, err := SeedFromEtcDefault(path)

			require.NoError(t, err)
			assert.Equal(t, path, res.Detail)
			assert.True(t, res.Found)
			assert.Equal(t, tt.enabled, res.Enabled)
		})
	}
}

func TestSeedFromEtcDefaultNotFound(t *testing.T) {
	tests := []struct {
		name    string
		content string
		missing bool
	}{
		{name: "missing file", missing: true},
		{name: "empty file", content: ""},
		{name: "only comments and blank lines", content: "# nothing here\n\n   \n"},
		{name: "unrelated keys only", content: "FOO=1\nDATADOG_SITE=datadoghq.com\n"},
		{name: "comment mentioning the variable is ignored", content: "# DATADOG_ENABLED=no is the old knob\n"},
		{name: "conditional block that never mentions the variable", content: "if [ -f /x ]; then\n  echo hi\nfi\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "datadog-agent-procmgr")
			if !tt.missing {
				require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o644))
			}

			res, err := SeedFromEtcDefault(path)

			require.NoError(t, err)
			assert.False(t, res.Found)
			assert.Equal(t, path, res.Detail, "Detail is the path even when the knob is absent")
		})
	}
}

func TestSeedFromEtcDefaultFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		// Lines that mention the variable but are not a simple assignment.
		{name: "spaces around equals", content: "DATADOG_ENABLED = no\n"},
		{name: "readonly", content: "readonly DATADOG_ENABLED=no\n"},
		{name: "declare", content: "declare DATADOG_ENABLED=no\n"},
		{name: "prefix assignment", content: "FOO=1 DATADOG_ENABLED=no\n"},
		{name: "conditional and", content: "[ -f /x ] && DATADOG_ENABLED=no\n"},
		{name: "export without whitespace", content: "exportDATADOG_ENABLED=no\n"},
		{name: "space after equals", content: "DATADOG_ENABLED= no\n"},
		{name: "similar variable name", content: "DATADOG_ENABLED_EXTRA=no\n"},
		// Conditional shell syntax around an assignment.
		{name: "indented assignment inside if then fi", content: "if [ -f /x ]; then\n  DATADOG_ENABLED=no\nfi\n"},
		{name: "assignment inside braces", content: "{\nDATADOG_ENABLED=no\n}\n"},
		{name: "assignment inside subshell", content: "(\nDATADOG_ENABLED=no\n)\n"},
		{name: "assignment inside case", content: "case $x in\na) DATADOG_ENABLED=no ;;\nesac\n"},
		// Quoting that does not parse as one plain value.
		{name: "quote followed by text", content: "DATADOG_ENABLED=\"no\"x\n"},
		{name: "lone double quote", content: "DATADOG_ENABLED=\"\n"},
		{name: "lone single quote", content: "DATADOG_ENABLED='\n"},
		{name: "unterminated quote", content: "DATADOG_ENABLED=\"no\n"},
		{name: "quote followed by hash without space", content: "DATADOG_ENABLED=\"no\"#off\n"},
		// Values that are not plain words.
		{name: "dollar expansion", content: "DATADOG_ENABLED=$VAR\n"},
		{name: "backticks", content: "DATADOG_ENABLED=`echo no`\n"},
		{name: "semicolon", content: "DATADOG_ENABLED=no;\n"},
		{name: "ampersand", content: "DATADOG_ENABLED=no&\n"},
		{name: "unquoted hash without space", content: "DATADOG_ENABLED=no#off\n"},
		{name: "quoted dollar", content: "DATADOG_ENABLED=\"$X\"\n"},
		// Continuation lines that reach the variable.
		{name: "backslash continuation before the assignment", content: "FOO=1 \\\nDATADOG_ENABLED=no\n"},
		// A malformed line is not skipped by a later valid one.
		{name: "malformed line then valid line", content: "DATADOG_ENABLED = no\nDATADOG_ENABLED=yes\n"},
		{name: "valid line then malformed line", content: "DATADOG_ENABLED=yes\nreadonly DATADOG_ENABLED=no\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeSeedFile(t, tt.content)

			res, err := SeedFromEtcDefault(path)

			var seedErr *SeedError
			require.ErrorAs(t, err, &seedErr)
			assert.Equal(t, path, seedErr.Detail)
			assert.Equal(t, path, res.Detail)
			assert.False(t, res.Found)
		})
	}
}

func TestSeedFromEtcDefaultReadErrors(t *testing.T) {
	t.Run("path is a directory", func(t *testing.T) {
		dir := t.TempDir()

		res, err := SeedFromEtcDefault(dir)

		var seedErr *SeedError
		require.ErrorAs(t, err, &seedErr)
		assert.Equal(t, dir, res.Detail)
	})
	t.Run("file larger than 4 MiB", func(t *testing.T) {
		path := writeSeedFile(t, "# "+strings.Repeat("x", maxSeedFileSize))

		_, err := SeedFromEtcDefault(path)

		var seedErr *SeedError
		require.ErrorAs(t, err, &seedErr)
	})
	t.Run("file of exactly 4 MiB is read", func(t *testing.T) {
		path := writeSeedFile(t, "DATADOG_ENABLED=no\n"+strings.Repeat("#", maxSeedFileSize-len("DATADOG_ENABLED=no\n")))

		res, err := SeedFromEtcDefault(path)

		require.NoError(t, err)
		assert.True(t, res.Found)
		assert.False(t, res.Enabled)
	})
}

func TestSeedFromEtcDefaultErrorUnwraps(t *testing.T) {
	path := writeSeedFile(t, "readonly DATADOG_ENABLED=no\n")

	_, err := SeedFromEtcDefault(path)

	var seedErr *SeedError
	require.True(t, errors.As(err, &seedErr))
	assert.Error(t, seedErr.Unwrap())
}

func TestEtcDefaultPathConstant(t *testing.T) {
	assert.Equal(t, "/etc/default/datadog-agent-procmgr", EtcDefaultPath)
}
