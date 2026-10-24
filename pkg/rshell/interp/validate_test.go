// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseScript is also defined in runner_test.go; it is redeclared here only
// if no shared test helper file exists yet.  If a compile error occurs,
// remove this duplicate and use the shared one.

func TestValidateRedirect_ReadOnlyMode(t *testing.T) {
	cases := []struct {
		script  string
		wantErr string
	}{
		{"echo x > file.txt", "> file redirection is not supported"},
		{"echo x >> file.txt", ">> file redirection is not supported"},
		{"echo x 2> file.txt", "> file redirection is not supported"},
		{"echo x 2>> file.txt", ">> file redirection is not supported"},
		{"echo x &> file.txt", "&> file redirection is not supported"},
		{"echo x &>> file.txt", "&>> file redirection is not supported"},
		{"echo x >| file.txt", "> file redirection is not supported"},
		{"echo x 3> file.txt", "3> fd redirection is not supported"},
		{"echo x 3>> file.txt", "3>> fd redirection is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.script, func(t *testing.T) {
			prog, err := ParseScript(tc.script, "")
			require.NoError(t, err)
			err = validateNode(prog, false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr, "unexpected error for %q", tc.script)
		})
	}
}

func TestValidateRedirect_RemediationMode(t *testing.T) {
	// These should pass validation (runtime sandbox enforces path policy).
	allowed := []string{
		"echo x > file.txt",
		"echo x >> file.txt",
		"echo x 2> file.txt",
		"echo x 2>> file.txt",
		"echo x &> file.txt",
		"echo x &>> file.txt",
		"echo x >| file.txt",
		"echo x >/dev/null",
	}
	for _, script := range allowed {
		t.Run(script, func(t *testing.T) {
			prog, err := ParseScript(script, "")
			require.NoError(t, err)
			assert.NoError(t, validateNode(prog, true), "expected no validation error for %q", script)
		})
	}

	// Unsupported fd redirects must be rejected even in remediation mode.
	unsupported := []struct {
		script  string
		wantErr string
	}{
		{"echo x 3> file.txt", "3> fd redirection is not supported"},
		{"echo x 3>> file.txt", "3>> fd redirection is not supported"},
		{"echo x 0> file.txt", "0> fd redirection is not supported"},
	}
	for _, tc := range unsupported {
		t.Run(tc.script, func(t *testing.T) {
			prog, err := ParseScript(tc.script, "")
			require.NoError(t, err)
			err = validateNode(prog, true)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateRedirect_AlwaysBlocked(t *testing.T) {
	cases := []struct {
		script  string
		wantErr string
	}{
		{"cat <> file.txt", "<> file redirection is not supported"},
		{"cat <<< hello", "<<< (herestring) is not supported"},
		{"cat <&0", "<&N fd duplication is not supported"},
		{"echo x >&3", ">&N fd duplication is not supported"},
	}
	for _, remMode := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(tc.script, func(t *testing.T) {
				prog, err := ParseScript(tc.script, "")
				require.NoError(t, err)
				err = validateNode(prog, remMode)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			})
		}
	}
}

func TestValidateNode_BlockedConstructs(t *testing.T) {
	cases := []struct {
		script  string
		wantErr string
	}{
		{"a[0]=1", "array index assignment is not supported"},
		{"${a[@]}", "array indexing is not supported"},
		{"let x=1", "let is not supported"},
		{"(( x++ ))", "arithmetic commands are not supported"},
		{"x=$(( 1+1 ))", "arithmetic expansion is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.script, func(t *testing.T) {
			prog, err := ParseScript(tc.script, "")
			require.NoError(t, err)
			err = validateNode(prog, false)
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tc.wantErr),
				"error %q does not contain %q", err.Error(), tc.wantErr)
		})
	}
}
