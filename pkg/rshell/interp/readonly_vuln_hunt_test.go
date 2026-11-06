// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// vuln-hunt campaign: 2026-05-18-initial-audit
// Target: readonly (shell-feature)
// These tests attempt to violate the readonly invariant: a variable marked
// ReadOnly via the Env() overlay must not be mutated by any script path.

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// runScriptWithReadonly creates a runner with RO_VAR set as a readonly
// variable in the writeEnv overlay and executes the given script.
func runScriptWithReadonly(t *testing.T, script string) (string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	r, err := New(StdIO(nil, &stdout, &stderr), allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	r.Reset()
	require.NoError(t, r.writeEnv.Set("RO_VAR", expand.Variable{
		Set:      true,
		Kind:     expand.String,
		Str:      "original",
		ReadOnly: true,
	}))

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	r.fillExpandConfig(context.Background())
	r.stmts(context.Background(), prog.Stmts)

	return stdout.String(), stderr.String()
}

// H3: Inline command variable override: `RO_VAR=hacked echo $RO_VAR`.
// The setVar call should fail with "readonly variable", the command should
// not run (because exit.ok() is false), and the restore path must NOT
// corrupt or downgrade the readonly flag.
func TestVulnHuntShellFeatureExpansionChain_InlineCmdVarReadonlyOverride(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"RO_VAR=hacked echo $RO_VAR\necho after=$RO_VAR\n")

	assert.Contains(t, stderr, "readonly variable",
		"inline cmd var assignment to readonly must produce readonly error")
	// `echo $RO_VAR` should NOT have run with the hacked value; if it did,
	// stdout would contain "hacked" before the after= line.
	assert.NotContains(t, stdout, "hacked",
		"the command must not run with the bypassed readonly value")
	assert.Contains(t, stdout, "after=original",
		"after the failed inline assignment, RO_VAR must remain 'original'")
}

// H5: `for RO_VAR in a b c; do echo $RO_VAR; done` — each iteration setVar fails,
// RO_VAR keeps its readonly value, body should observe the original.
func TestVulnHuntShellFeatureExpansionChain_ForLoopReadonlyTarget(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"for RO_VAR in a b c; do echo $RO_VAR; done\necho after=$RO_VAR\n")

	assert.Contains(t, stderr, "readonly variable",
		"for-loop assignment to readonly must produce readonly error")
	// In bash, for X in a b c with X readonly aborts with an error and
	// RO_VAR keeps its original value. In rshell, the assignment fails and
	// the body should not run with a hacked value.
	assert.NotContains(t, stdout, "\na\n", "iteration must not bind RO_VAR=a")
	assert.NotContains(t, stdout, "\nb\n", "iteration must not bind RO_VAR=b")
	assert.NotContains(t, stdout, "\nc\n", "iteration must not bind RO_VAR=c")
	assert.Contains(t, stdout, "after=original",
		"after the for loop, RO_VAR must remain 'original'")
}

// H6: `read RO_VAR` builtin assignment. The read builtin must surface the
// readonly error and leave RO_VAR unchanged.
func TestVulnHuntShellFeatureExpansionChain_ReadBuiltinReadonlyTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("hacked\n")
	r, err := New(StdIO(stdin, &stdout, &stderr), allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	r.Reset()
	require.NoError(t, r.writeEnv.Set("RO_VAR", expand.Variable{
		Set:      true,
		Kind:     expand.String,
		Str:      "original",
		ReadOnly: true,
	}))

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("read RO_VAR\necho after=$RO_VAR\n"), "")
	require.NoError(t, err)

	r.fillExpandConfig(context.Background())
	r.stmts(context.Background(), prog.Stmts)

	assert.Contains(t, stderr.String(), "readonly variable",
		"read RO_VAR must produce a readonly error on stderr")
	assert.Contains(t, stdout.String(), "after=original",
		"read must not bypass readonly; RO_VAR must remain 'original'")
	assert.NotContains(t, stdout.String(), "hacked",
		"RO_VAR must not be updated to read value")
}

// H7: Multi-variable inline assignment with a readonly target mixed in.
// `FOO=ok RO_VAR=evil cmd` — the first setVar succeeds, the second fails.
// Concern: the restore defer uses setUncapped which does NOT check ReadOnly.
// Both entries are in restores; verify nothing leaks and FOO is restored.
func TestVulnHuntShellFeatureReadonlyBypass_MultiVarInlineWithReadonly(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"FOO=ok RO_VAR=evil echo HIT\necho after foo=$FOO ro=$RO_VAR\n")

	assert.Contains(t, stderr, "readonly variable",
		"the readonly target must produce a readonly error")
	assert.NotContains(t, stdout, "HIT",
		"the command itself must not execute after a failed inline assignment")
	// FOO did not exist before; after the failed run, FOO must not leak as set.
	assert.Contains(t, stdout, "after foo= ro=original",
		"FOO must be restored (to unset) and RO_VAR must remain 'original'")
}

// H10: `( RO_VAR=hacked; echo $RO_VAR )` — non-background subshell.
// The subshell overlay parent-points at the outer overlay, so prev.ReadOnly
// must read as true through the parent chain.
func TestVulnHuntShellFeatureSubshellIsolation_ParenSubshellAttack(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"( RO_VAR=hacked; echo $RO_VAR )\necho after=$RO_VAR\n")

	assert.Contains(t, stderr, "readonly variable",
		"subshell assignment to readonly must produce readonly error")
	// $RO_VAR inside the subshell should still be 'original'.
	assert.NotContains(t, stdout, "hacked",
		"$RO_VAR must remain 'original' even inside the subshell")
	assert.Contains(t, stdout, "after=original",
		"$RO_VAR must remain 'original' in the parent after the subshell")
}

// H11: Right side of a pipe is a background subshell. When background=true,
// newOverlayEnviron copies all parent variables into o.values (including the
// readonly flag). Verify the readonly flag survives the copy.
func TestVulnHuntShellFeatureSubshellIsolation_PipeRightReadonlyOverride(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"echo seed | { RO_VAR=hacked; echo inside=$RO_VAR; }\necho after=$RO_VAR\n")

	assert.Contains(t, stderr, "readonly variable",
		"pipe-right-side assignment to readonly must produce readonly error")
	assert.NotContains(t, stdout, "inside=hacked",
		"pipe right side must not see a bypassed readonly value")
	assert.Contains(t, stdout, "after=original",
		"$RO_VAR must remain 'original' in the parent after the pipeline")
}

// H11b: Right side of a pipe is a Subshell `(...)` rather than a Block `{...}`.
func TestVulnHuntShellFeatureSubshellIsolation_PipeRightParenSubshell(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"echo seed | ( RO_VAR=hacked; echo inside=$RO_VAR; )\necho after=$RO_VAR\n")

	assert.Contains(t, stderr, "readonly variable",
		"pipe-right-side `(...)` assignment to readonly must produce readonly error")
	assert.NotContains(t, stdout, "inside=hacked",
		"pipe right side `(...)` must not see a bypassed readonly value")
	assert.Contains(t, stdout, "after=original",
		"$RO_VAR must remain 'original' in the parent after the pipeline")
}

// H12: Command substitution: `$(RO_VAR=hacked; echo $RO_VAR)` runs in a subshell
// with its own writeEnv overlay; readonly must propagate.
func TestVulnHuntShellFeatureSubshellIsolation_CmdSubstReadonlyOverride(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"echo got=$(RO_VAR=hacked; echo $RO_VAR)\necho after=$RO_VAR\n")

	assert.Contains(t, stderr, "readonly variable",
		"cmd-sub assignment to readonly must produce readonly error")
	assert.NotContains(t, stdout, "got=hacked",
		"cmd-sub must not yield the bypassed readonly value")
	assert.Contains(t, stdout, "after=original",
		"$RO_VAR must remain 'original' in the parent after cmd-sub")
}

// H1: `readonly X=5 > /dev/null` — readonly is blocked at parse validation
// which runs before any redirect setup. Verify the redirect does not bypass.
func TestVulnHuntShellFeatureRedirectionChain_ReadonlyWithRedirect(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r, err := New(StdIO(nil, &stdout, &stderr), allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	r.Reset()

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("readonly X=5 > /dev/null\n"), "")
	require.NoError(t, err)

	r.fillExpandConfig(context.Background())
	exit := r.Run(context.Background(), prog)
	_ = exit
	assert.Contains(t, stderr.String(), "readonly is not supported",
		"readonly must be blocked at parse validation regardless of redirect")
}

// H13: Sibling DeclClause variants `typeset -r X=5` and `nameref X=Y` must
// also be blocked (declare/local/export/readonly are already covered by
// existing blocked_commands scenarios).
func TestVulnHuntShellFeatureParserConfusion_DeclClauseTypesetNameref(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		want   string
	}{
		{"typeset", "typeset -r X=5\n", "typeset is not supported"},
		{"nameref", "nameref Y=X\n", "nameref is not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			r, err := New(StdIO(nil, &stdout, &stderr), allowAllCommandsOpt())
			require.NoError(t, err)
			t.Cleanup(func() { r.Close() })
			r.Reset()

			parser := syntax.NewParser()
			prog, perr := parser.Parse(strings.NewReader(tc.script), "")
			if perr != nil {
				// Some variants may not lex as DeclClause depending on the
				// parser dialect; record the parse error as a different kind
				// of block.
				t.Logf("parse error for %s: %v", tc.name, perr)
				return
			}
			r.fillExpandConfig(context.Background())
			_ = r.Run(context.Background(), prog)
			assert.Contains(t, stderr.String(), tc.want,
				"%s must be blocked", tc.name)
		})
	}
}

// H14: Escaped or quoted `readonly` is parsed as a regular CallExpr, not a
// DeclClause. It is then a missing command (rshell has no `readonly`
// command), so it fails with command-not-found. Verify it does NOT succeed
// in marking X as readonly.
func TestVulnHuntShellFeatureParserConfusion_QuotedReadonlyKeyword(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
	}{
		{"backslash", `\readonly X=5` + "\n" + `echo after=$X` + "\n"},
		{"single_quoted", `'readonly' X=5` + "\n" + `echo after=$X` + "\n"},
		{"double_quoted", `"readonly" X=5` + "\n" + `echo after=$X` + "\n"},
		{"fragmented", `read""only X=5` + "\n" + `echo after=$X` + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			r, err := New(StdIO(nil, &stdout, &stderr), allowAllCommandsOpt())
			require.NoError(t, err)
			t.Cleanup(func() { r.Close() })
			r.Reset()

			parser := syntax.NewParser()
			prog, perr := parser.Parse(strings.NewReader(tc.script), "")
			require.NoError(t, perr)
			r.fillExpandConfig(context.Background())
			_ = r.Run(context.Background(), prog)
			// Either the script blocked at validation (still recognized as a
			// DeclClause), or it executed as a missing command. Either way,
			// X must NOT be observable as a writable variable assigned to 5
			// being treated as readonly.
			// In bash, `\readonly X=5` would execute the readonly command;
			// in rshell, no such command exists, so X assignment is a normal
			// assignment that may succeed as a normal write (no readonly bit).
			out := stdout.String()
			if strings.Contains(out, "after=5") {
				// A regular CallExpr with command=`readonly` and assignments
				// has bash semantics that would mark X readonly. In rshell,
				// the assignment may have leaked. This is acceptable IFF X
				// was not marked readonly (no readonly bit), which we cannot
				// directly check from the script, but we can confirm by
				// trying to reassign.
				// Reassign and check.
				r2 := r
				var out2, err2 bytes.Buffer
				r2.stdout = &out2
				r2.stderr = &err2
				prog2, perr2 := parser.Parse(strings.NewReader("X=10\necho check=$X\n"), "")
				require.NoError(t, perr2)
				_ = r2.Run(context.Background(), prog2)
				assert.Contains(t, out2.String(), "check=10",
					"%s: X must not have a sticky readonly bit", tc.name)
			}
		})
	}
}

// H15: `echo $(readonly X=5)` — readonly inside cmd-sub must be detected by
// the validator (which walks the entire AST including CmdSubst bodies).
func TestVulnHuntShellFeatureParserConfusion_CmdSubstReadonly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r, err := New(StdIO(nil, &stdout, &stderr), allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	r.Reset()

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("echo $(readonly X=5; echo X=$X)\n"), "")
	require.NoError(t, err)
	r.fillExpandConfig(context.Background())
	_ = r.Run(context.Background(), prog)
	assert.Contains(t, stderr.String(), "readonly is not supported",
		"readonly nested in $() must still be blocked by validateNode")
}

// H4: `${RO_VAR:=hacked}` parameter expansion that would assign on unset/null.
// validateParamExp blocks `pe.Exp != nil` at parse, so this must fail with
// "${var} operations ... are not supported".
func TestVulnHuntShellFeatureExpansionChain_DefaultAssignmentExpansion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r, err := New(StdIO(nil, &stdout, &stderr), allowAllCommandsOpt())
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	r.Reset()
	require.NoError(t, r.writeEnv.Set("RO_VAR", expand.Variable{
		Set:      true,
		Kind:     expand.String,
		Str:      "original",
		ReadOnly: true,
	}))

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(`echo ${RO_VAR:=hacked}`+"\n"), "")
	require.NoError(t, err)
	r.fillExpandConfig(context.Background())
	_ = r.Run(context.Background(), prog)
	assert.Contains(t, stderr.String(), "not supported",
		"${var:=value} parameter expansion must be blocked at parse validation")
	assert.NotContains(t, stdout.String(), "hacked",
		"the would-be assignment must not produce 'hacked' in stdout")
}
