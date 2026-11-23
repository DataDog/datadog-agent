// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"context"
	"strings"
	"testing"
	"time"

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
			err = validateNode(context.Background(), prog, false)
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
			assert.NoError(t, validateNode(context.Background(), prog, true), "expected no validation error for %q", script)
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
			err = validateNode(context.Background(), prog, true)
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
				err = validateNode(context.Background(), prog, remMode)
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
			err = validateNode(context.Background(), prog, false)
			require.Error(t, err)
			assert.True(t, strings.Contains(err.Error(), tc.wantErr),
				"error %q does not contain %q", err.Error(), tc.wantErr)
		})
	}
}

// Brace expansion runs before tilde expansion, so a "~" that brace expansion
// moves to the start of a field must be rejected like a leading "~".
func TestValidateTildeViaBraceExpansion(t *testing.T) {
	blocked := []string{
		"echo {~root/,x}",
		"echo {x,~root}",
		"echo {,}~root",
		"echo {,x}~root/",
		"echo {{~root,a},b}",
		"echo {a,{,}}~root",
		`echo {"",}~root`,
		"{,}~root",
		"for x in {~root,}; do echo $x; done",
	}
	for _, script := range blocked {
		t.Run(script, func(t *testing.T) {
			prog, err := ParseScript(script, "")
			require.NoError(t, err)
			err = validateNode(context.Background(), prog, false)
			require.Error(t, err)
			assert.Equal(t, "tilde expansion is not supported", err.Error())
		})
	}

	allowed := []string{
		"echo {a,b}",
		"echo {1..3} {a..c}",
		"echo {a,b}~root",
		"echo x{~root,}",
		`echo {"a",b}~root`,
		`echo {$x,y}~root`,
		`echo "{~root,}" '{,}~root'`,
		`echo \~root {a,\~b}`,
		// Assignment values and heredoc delimiters never undergo brace
		// expansion, so a "~" inside their braces is never a tilde prefix.
		`x={~root/,x}; echo "$x"`,
		"x={,}~root true",
		"cat <<{~root/,x}\nok\n{~root/,x}\n",
		"cat <<-{,}~root\nok\n{,}~root\n",
	}
	for _, script := range allowed {
		t.Run(script, func(t *testing.T) {
			prog, err := ParseScript(script, "")
			require.NoError(t, err)
			assert.NoError(t, validateNode(context.Background(), prog, false))
		})
	}
}

// A leading "~" is still rejected in assignment values and heredoc delimiters,
// and command substitutions inside assignment values are still validated.
func TestValidateTildeLiteralWords(t *testing.T) {
	for _, script := range []string{
		"x=~root",
		"cat <<~root\nok\n~root\n",
		"x=$(echo {~root,})",
	} {
		t.Run(script, func(t *testing.T) {
			prog, err := ParseScript(script, "")
			require.NoError(t, err)
			err = validateNode(context.Background(), prog, false)
			require.Error(t, err)
			assert.Equal(t, "tilde expansion is not supported", err.Error())
		})
	}
}

func nestedBraceWord(depth int, inner string) string {
	return strings.Repeat("{a,", depth) + inner + strings.Repeat("}", depth)
}

// Deeply nested braces in a dormant branch must not make validation
// superlinear. Without a "~" the word is not brace-analysed at all; with one,
// nesting beyond maxBraceTildeDepth is rejected without analysis.
func TestValidateTildeNestedBraceScaling(t *testing.T) {
	const depth = 20000
	cases := []struct {
		script  string
		wantErr string
	}{
		{"if false; then echo " + nestedBraceWord(depth, "b") + "; fi", ""},
		{"if false; then echo " + nestedBraceWord(depth, "~root") + "; fi",
			"tilde expansion is not supported (brace nesting exceeds 64)"},
		{"if false; then echo " + strings.Repeat("{", depth) + "~root; fi",
			"tilde expansion is not supported (brace nesting exceeds 64)"},
		{"if false; then echo " + nestedBraceWord(maxBraceTildeDepth, "b") + "~root; fi", ""},
		{"if false; then echo " + nestedBraceWord(maxBraceTildeDepth, "~root") + "; fi",
			"tilde expansion is not supported"},
	}
	for i, tc := range cases {
		prog, err := ParseScript(tc.script, "")
		require.NoError(t, err)
		start := time.Now()
		err = validateNode(context.Background(), prog, false)
		elapsed := time.Since(start)
		if tc.wantErr == "" {
			assert.NoError(t, err, "case %d", i)
		} else if assert.Error(t, err, "case %d", i) {
			assert.Equal(t, tc.wantErr, err.Error(), "case %d", i)
		}
		assert.Less(t, elapsed, time.Second, "case %d: validation took %v", i, elapsed)
	}
}

// Brace analysis honours cancellation.
func TestValidateNodeHonoursCancellation(t *testing.T) {
	prog, err := ParseScript("echo "+strings.Repeat("{,}", 4096)+"~root", "")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, validateNode(ctx, prog, false), context.Canceled)
}
