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

func TestReadonlyVariableBlocksReassignment(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r, err := New(
		StdIO(nil, &stdout, &stderr),
		Env("RO_VAR=original"),
		allowAllCommandsOpt(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	// Mark RO_VAR as readonly via the environment overlay.
	r.Reset()
	r.writeEnv.Set("RO_VAR", expand.Variable{
		Set:      true,
		Kind:     expand.String,
		Str:      "original",
		ReadOnly: true,
	})

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("RO_VAR=changed\necho $RO_VAR"), "")
	require.NoError(t, err)

	r.fillExpandConfig(context.Background())
	r.stmts(context.Background(), prog.Stmts)

	assert.Contains(t, stderr.String(), "readonly variable",
		"reassigning a readonly variable should produce an error on stderr")
	assert.Contains(t, stdout.String(), "original",
		"readonly variable value should remain unchanged")
}

func TestReadonlyVariableBlocksUnset(t *testing.T) {
	r := newResetRunner(t)

	// Set a readonly variable.
	r.writeEnv.Set("RO_VAR", expand.Variable{
		Set:      true,
		Kind:     expand.String,
		Str:      "protected",
		ReadOnly: true,
	})

	// Attempt to unset it by passing an unset Variable.
	err := r.writeEnv.Set("RO_VAR", expand.Variable{})
	assert.Error(t, err, "unsetting a readonly variable should return an error")
	assert.Contains(t, err.Error(), "readonly variable")

	// Verify the variable is still set.
	vr := r.writeEnv.Get("RO_VAR")
	assert.Equal(t, "protected", vr.Str, "readonly variable should still hold its value")
}

func TestReadonlyVariableBlocksKeepValueAttributeChange(t *testing.T) {
	r := newResetRunner(t)

	// Set a readonly variable.
	r.writeEnv.Set("RO_VAR", expand.Variable{
		Set:      true,
		Kind:     expand.String,
		Str:      "locked",
		ReadOnly: true,
	})

	// Attempt to change attributes via KeepValue.
	err := r.writeEnv.Set("RO_VAR", expand.Variable{
		Kind:     expand.KeepValue,
		Exported: true,
	})
	assert.Error(t, err, "modifying attributes on a readonly variable via KeepValue should fail")
	assert.Contains(t, err.Error(), "readonly variable")
}
