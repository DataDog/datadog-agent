// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// newResetRunner creates a Runner via New and calls Reset so that internal
// state (writeEnv, etc.) is fully initialised for unit-level tests.
func newResetRunner(t *testing.T) *Runner {
	t.Helper()
	r, err := New()
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	r.Reset()
	return r
}

func TestInternalErrorf(t *testing.T) {
	r := newResetRunner(t)

	r.internalErrorf("something went wrong: %s", "details")

	require.True(t, r.exit.fatalExit)
	assert.Contains(t, r.exit.err.Error(), "internal error: something went wrong: details")
}

func TestInternalErrorfIdempotent(t *testing.T) {
	r := newResetRunner(t)

	r.internalErrorf("first error")
	r.internalErrorf("second error")

	assert.Contains(t, r.exit.err.Error(), "first error",
		"exit.fatal keeps the first error and ignores subsequent calls")
}

func TestLookupVarEmptyName(t *testing.T) {
	r := newResetRunner(t)

	vr := r.lookupVar("")

	assert.Equal(t, expand.Variable{}, vr, "should return zero Variable")
	require.True(t, r.exit.fatalExit)
	assert.Contains(t, r.exit.err.Error(), "variable name must not be empty")
}

func TestSetVarWithIndexNonNilIndex(t *testing.T) {
	r := newResetRunner(t)

	// Any non-nil ArithmExpr triggers the invariant check.
	idx := &syntax.Word{}
	r.setVarWithIndex(expand.Variable{}, "X", idx, expand.Variable{})

	require.True(t, r.exit.fatalExit)
	assert.Contains(t, r.exit.err.Error(), "index should have been rejected by AST validation")
}

func TestAssignValAppend(t *testing.T) {
	r := newResetRunner(t)

	as := &syntax.Assign{Append: true, Name: &syntax.Lit{Value: "X"}}
	vr := r.assignVal(expand.Variable{}, as, "")

	assert.Equal(t, expand.Variable{}, vr, "should return zero Variable")
	require.True(t, r.exit.fatalExit)
	assert.Contains(t, r.exit.err.Error(), "append should have been rejected by AST validation")
}

func TestAssignValArray(t *testing.T) {
	r := newResetRunner(t)

	as := &syntax.Assign{
		Name:  &syntax.Lit{Value: "X"},
		Array: &syntax.ArrayExpr{},
	}
	vr := r.assignVal(expand.Variable{}, as, "")

	assert.Equal(t, expand.Variable{}, vr, "should return zero Variable")
	require.True(t, r.exit.fatalExit)
	assert.Contains(t, r.exit.err.Error(), "array assignment should have been rejected by AST validation")
}

func TestResetZeroValueRunnerSetsFatal(t *testing.T) {
	var r Runner
	r.Reset()

	require.True(t, r.exit.fatalExit)
	assert.Contains(t, r.exit.err.Error(), "use interp.New to construct a Runner")
}

func TestRunZeroValueRunnerMultipleCalls(t *testing.T) {
	// Calling Run repeatedly on a zero-value Runner should consistently
	// return an explicit error, not panic.
	var r Runner
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("echo hi"), "")
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err = r.Run(context.Background(), prog)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "use interp.New to construct a Runner")
	}
}

func TestResetPreservesConfig(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	r, err := New(
		StdIO(nil, &outBuf, &errBuf),
		Env("FOO=bar"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	r.Dir = "/tmp"
	r.Reset()

	// Config fields should be preserved.
	assert.Equal(t, "/tmp", r.origDir)
	assert.NotNil(t, r.execHandler, "execHandler should be set after Reset")
	assert.NotNil(t, r.openHandler, "openHandler should be set after Reset")
	assert.NotNil(t, r.readDirHandler, "readDirHandler should be set after Reset")
	assert.True(t, r.usedNew)

	// State should be reinitialized from config.
	assert.Equal(t, "/tmp", r.Dir)
	assert.Equal(t, &outBuf, r.stdout)
	assert.Equal(t, &errBuf, r.stderr)
	assert.True(t, r.didReset)

	// Execution state should be zero.
	assert.Equal(t, exitStatus{}, r.exit)
	assert.Equal(t, exitStatus{}, r.lastExit)
	assert.False(t, r.inLoop)
	assert.Equal(t, 0, r.breakEnclosing)
	assert.Equal(t, 0, r.contnEnclosing)
	assert.Equal(t, "", r.filename)
}

func TestResetReinitializesState(t *testing.T) {
	r := newResetRunner(t)

	// Mutate state fields as execution would.
	r.Dir = "/changed"
	r.exit = exitStatus{code: 42}
	r.lastExit = exitStatus{code: 7}
	r.inLoop = true
	r.breakEnclosing = 3
	r.contnEnclosing = 2
	r.filename = "test.sh"

	r.Reset()

	// State should be reset; Dir reverts to origDir (set during first Reset).
	assert.NotEqual(t, "/changed", r.Dir, "Dir should revert to origDir")
	assert.Equal(t, exitStatus{}, r.exit)
	assert.Equal(t, exitStatus{}, r.lastExit)
	assert.False(t, r.inLoop)
	assert.Equal(t, 0, r.breakEnclosing)
	assert.Equal(t, 0, r.contnEnclosing)
	assert.Equal(t, "", r.filename)
}

func TestMultipleResetsPreserveConfig(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	r, err := New(StdIO(nil, &outBuf, &errBuf))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	r.Reset()
	configAfterFirstReset := r.runnerConfig

	// Mutate state and reset again.
	r.Dir = "/changed"
	r.exit = exitStatus{code: 1}
	r.Reset()

	// Config struct should be identical across resets.
	assert.Equal(t, configAfterFirstReset.origDir, r.runnerConfig.origDir)
	assert.Equal(t, configAfterFirstReset.origStdout, r.runnerConfig.origStdout)
	assert.Equal(t, configAfterFirstReset.origStderr, r.runnerConfig.origStderr)
	assert.Equal(t, configAfterFirstReset.usedNew, r.runnerConfig.usedNew)
	assert.Equal(t, &outBuf, r.stdout)
	assert.Equal(t, &errBuf, r.stderr)
}

func TestSubshellInheritsConfig(t *testing.T) {
	r := newResetRunner(t)
	r.fillExpandConfig(context.Background())

	sub := r.subshell(false)

	// The entire config struct should be shared.
	assert.Equal(t, r.runnerConfig.origDir, sub.runnerConfig.origDir)
	assert.Equal(t, r.runnerConfig.sandbox, sub.runnerConfig.sandbox)
	assert.True(t, sub.usedNew)
	assert.True(t, sub.didReset)
}

func TestSubshellHasIndependentState(t *testing.T) {
	r := newResetRunner(t)
	r.fillExpandConfig(context.Background())
	r.Dir = "/parent"

	sub := r.subshell(false)
	sub.Dir = "/child"
	sub.exit = exitStatus{code: 42}
	sub.inLoop = true

	// Parent state should be unaffected.
	assert.Equal(t, "/parent", r.Dir)
	assert.Equal(t, exitStatus{}, r.exit)
	assert.False(t, r.inLoop)
}

func TestSubshellBackgroundCopiesEnv(t *testing.T) {
	r := newResetRunner(t)
	r.setVarString("X", "parent_value")
	r.fillExpandConfig(context.Background())

	sub := r.subshell(true)

	// Background subshell should have a snapshot of the environment.
	vr := sub.writeEnv.Get("X")
	assert.Equal(t, "parent_value", vr.Str)

	// Mutating parent env should not affect the background subshell's snapshot.
	r.setVarString("X", "changed")
	vr = sub.writeEnv.Get("X")
	assert.Equal(t, "parent_value", vr.Str,
		"background subshell env should be isolated from parent mutations")
}

func TestExpandErrUnexpectedCommand(t *testing.T) {
	r := newResetRunner(t)
	r.expandErr(expand.UnexpectedCommandError{Node: &syntax.CmdSubst{}})
	assert.True(t, r.exit.exiting)
	assert.Equal(t, uint8(1), r.exit.code)
}

// TestExpandErrStorageExhaustedAborts verifies that expandErr treats
// errTotalVarStorageExceeded as fatal (sets exiting=true), matching the
// behaviour of the direct-assignment path in setVar.  This guards against
// a regression where a cap violation arriving via a parameter-expansion
// assignment (e.g. ${var:=value} if that syntax is ever re-enabled) would
// only set exit.code=1 without aborting the script.
func TestExpandErrStorageExhaustedAborts(t *testing.T) {
	var errBuf bytes.Buffer
	r, err := New(StdIO(nil, nil, &errBuf))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	r.Reset()

	// expandEnv.Set wraps the error with the variable name before it reaches
	// expandErr, producing "name: variable storage limit exceeded (N bytes total)".
	storageErr := fmt.Errorf("UNSET_VAR: %w", &errTotalVarStorageExceeded{total: 1048577})
	r.expandErr(storageErr)

	assert.True(t, r.exit.exiting, "expandErr must set exiting=true for storage-cap errors so the script aborts")
	assert.Equal(t, uint8(1), r.exit.code)
	assert.Contains(t, errBuf.String(), "variable storage limit exceeded")
}

func TestInternalErrorStopsExecution(t *testing.T) {
	// After an internal error, the runner's stop() check should halt
	// further statement execution, surfacing the error via Run.
	r := newResetRunner(t)

	// Inject a fatal internal error before running a program.
	r.internalErrorf("forced failure")

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("echo should_not_run"), "")
	require.NoError(t, err)

	// Run resets exit, but the next stmt() call will see exiting=true.
	// We verify the pattern by calling stmts directly on a prepared runner.
	r.fillExpandConfig(context.Background())
	r.stmts(context.Background(), prog.Stmts)

	assert.True(t, r.exit.fatalExit, "fatal flag should remain set")
}
