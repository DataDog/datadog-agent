// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

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

func strVar(s string) expand.Variable {
	return expand.Variable{Set: true, Kind: expand.String, Str: s}
}

// TestVarBudgetSharedByConcurrentOverlays checks the budget directly: two
// sibling overlays cannot together exceed the cap, and a released sibling's
// storage becomes available again.
func TestVarBudgetSharedByConcurrentOverlays(t *testing.T) {
	root := &overlayEnviron{}
	root.resetAccounting()
	require.NoError(t, root.Set("BASE", strVar(strings.Repeat("b", 100<<10))))

	left := newOverlayEnviron(root, true)
	right := newOverlayEnviron(root, true)
	half := strings.Repeat("x", 500<<10)
	require.NoError(t, left.Set("X", strVar(half)))
	err := right.Set("X", strVar(half))
	var storageErr *errTotalVarStorageExceeded
	require.ErrorAs(t, err, &storageErr, "siblings must share one budget")

	left.release()
	require.NoError(t, right.Set("X", strVar(half)), "released storage must be reusable")

	// A subshell of a stage is charged against its ancestors' and its
	// siblings' storage through the shared budget.
	nested := newOverlayEnviron(right, false)
	assert.ErrorAs(t, nested.Set("Y", strVar(half)), &storageErr)
	require.NoError(t, nested.Set("Y", strVar(strings.Repeat("y", 100<<10))))
	nested.release()
	right.release()
	assert.Equal(t, root.charged, root.budget.live)
}

// TestVarBudgetReturnsToRootAfterRun checks that every subshell and pipeline
// stage releases what it charged, so only the root's own storage remains.
func TestVarBudgetReturnsToRootAfterRun(t *testing.T) {
	script := `A=1
x=$(B=2; echo $B)
( C=3 )
{ D=4; echo $D; } | { E=5; cat; } | { F=$(G=6; echo $G); cat; }
for i in 1 2 3; do H=$i | cat; done
I=7 J=$I true
`
	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	require.NoError(t, err)
	var out bytes.Buffer
	r, err := New(StdIO(nil, &out, &out), AllowedCommands([]string{"rshell:echo", "rshell:cat", "rshell:true"}))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	require.NoError(t, r.Run(context.Background(), prog), out.String())

	root := r.writeEnv.(*overlayEnviron)
	assert.Equal(t, root.charged, root.budget.live)
}

// TestVarBudgetInheritedStorageNotRefunded checks that hiding or replacing a
// variable an ancestor still holds does not refund the ancestor's storage.
func TestVarBudgetInheritedStorageNotRefunded(t *testing.T) {
	big := strings.Repeat("a", 600<<10)
	other := strings.Repeat("b", 600<<10)
	for name, background := range map[string]bool{"subshell": false, "pipeline stage": true} {
		t.Run(name, func(t *testing.T) {
			var storageErr *errTotalVarStorageExceeded

			root := &overlayEnviron{}
			root.resetAccounting()
			require.NoError(t, root.Set("A", strVar(big)))

			replace := newOverlayEnviron(root, background)
			assert.ErrorAs(t, replace.Set("A", strVar(other)), &storageErr,
				"replacing A must not refund the parent's copy")
			replace.release()

			clear := newOverlayEnviron(root, background)
			require.NoError(t, clear.Set("A", strVar("")))
			assert.ErrorAs(t, clear.Set("B", strVar(other)), &storageErr,
				"clearing A must not refund the parent's copy")
			clear.release()

			assert.Equal(t, root.charged, root.budget.live)
		})
	}
}

// TestVarBudgetTemporaryAssignmentsRestoreExactly checks that the binding
// before FOO=val cmd comes back exactly: a variable that did not exist leaves
// no entry behind (an undeclared entry would cost nothing yet still occupy
// the map), an inherited value is not copied into the child overlay, and the
// overlay owns exactly what it owned before.
func TestVarBudgetTemporaryAssignmentsRestoreExactly(t *testing.T) {
	script := `A=x
NEW1= true
NEW2=v true
A=y true
( A=z true; INNER= true; echo "$A" )
`
	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	require.NoError(t, err)
	var out bytes.Buffer
	r, err := New(StdIO(nil, &out, &out), AllowedCommands([]string{"rshell:echo", "rshell:true"}))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	require.NoError(t, r.Run(context.Background(), prog), out.String())
	assert.Equal(t, "x\n", out.String())

	root := r.writeEnv.(*overlayEnviron)
	for _, name := range []string{"NEW1", "NEW2", "INNER"} {
		_, ok := root.values[name]
		assert.False(t, ok, "%s left an entry", name)
		_, ok = root.owned[name]
		assert.False(t, ok, "%s left owned storage", name)
	}
	assert.Equal(t, strVar("x").Str, root.values["A"].Str)
	assert.Equal(t, varStorageCost("A", root.values["A"]), root.owned["A"])
	assert.Equal(t, root.charged, root.budget.live)

	// A child overlay restoring an inherited binding hides nothing and owns
	// nothing once the command ends.
	child := newOverlayEnviron(root, false)
	snap := child.snapshotBinding("A")
	require.NoError(t, child.Set("A", strVar(strings.Repeat("c", 1024))))
	child.restoreBinding(snap)
	_, ok := child.values["A"]
	assert.False(t, ok)
	assert.Equal(t, varStorage{}, child.charged)
	assert.Equal(t, "x", child.Get("A").Str)
	child.release()
	assert.Equal(t, root.charged, root.budget.live)
}
