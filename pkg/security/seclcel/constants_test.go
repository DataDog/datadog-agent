// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// TestConstants covers the names a rule uses for the kernel's own constants, which a
// fifth of the real rules need and none of them could use before.
func TestConstants(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.Open.Flags = 577 // O_WRONLY|O_CREAT|O_TRUNC
	event.Open.File.BasenameStr = "passwd"

	tests := []struct {
		expr string
		want bool
	}{
		// the flags idiom, which is what most of them are for
		{`open.flags & O_CREAT > 0`, true},
		{`open.flags & O_APPEND > 0`, false},
		{`open.flags & (O_CREAT|O_RDWR|O_WRONLY) > 0`, true},
		{`open.flags & (O_RDONLY|O_DIRECTORY) > 0`, false},
		// a constant on its own, and in a list
		{`open.flags == 577`, true},
		{`open.flags & O_TRUNC == O_TRUNC`, true},
		{`open.file.name in [ "shadow", "passwd" ]`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			assert.Equal(t, tt.want, evalSECL(t, env, event, tt.expr))
		})
	}
}

// TestConstantsAreTyped is the other half of declaring them: a constant carries the
// type the model gave it, so using one where it does not belong is a rule that fails
// to compile rather than a rule that never matches.
func TestConstantsAreTyped(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	_, err = NewRule(env, `open.file.name == O_CREAT`, ModelFieldTypes{})
	require.Error(t, err, "a string field compared against an integer constant")
	assert.Contains(t, err.Error(), "no matching overload")
}

// TestMixedConstantListIsAccepted records a divergence rather than a guarantee.
//
// SECL refuses to mix constant types in a list — "can't mix constants types in arrays"
// (eval.go:253) — where CEL widens a heterogeneous list literal to list(dyn) and
// accepts membership against it, then finds no element that matches. So a rule SECL
// rejects at compile time is here a rule that never fires.
//
// It is not worth rejecting ourselves: the translation would have to type the list
// before the checker does, and a rule this confused is caught by the SECL side of any
// policy that loads through both.
func TestMixedConstantListIsAccepted(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.Open.Flags = 577

	assert.False(t, evalSECL(t, env, event, `open.flags in [ O_CREAT, "S_IFREG" ]`),
		"a mixed list compiles and matches nothing")
}

// TestBooleanConstantsStayLiterals pins the one entry of the table that is not
// declared: `true` and `false` are CEL keywords, so declaring them would be rejected,
// and the translation emits them as literals instead.
func TestBooleanConstantsStayLiterals(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.Open.File.BasenameStr = "passwd"

	assert.True(t, evalSECL(t, env, event, `true && open.file.name == "passwd"`))
	assert.False(t, evalSECL(t, env, event, `false || open.file.name == "shadow"`))

	// And the model does hold them, which is why they have to be skipped rather than
	// simply absent from the tables.
	assert.Contains(t, model.SECLConstants(), "true")
}

// TestStringArrayConstant covers the one constant that is not an integer, so that a
// second one appearing does not go unnoticed.
func TestStringArrayConstant(t *testing.T) {
	if _, ok := model.SECLConstants()["CWS_MAP_NAMES"]; !ok {
		t.Skip("CWS_MAP_NAMES is a linux constant")
	}

	env, err := NewModelEnv()
	require.NoError(t, err)

	// It type-checks as a list of strings, which is what a rule needs it for.
	_, err = NewRule(env, `bpf.map.name in CWS_MAP_NAMES`, ModelFieldTypes{})
	require.NoError(t, err)
}
