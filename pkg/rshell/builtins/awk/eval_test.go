// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func TestArrayLengthDoesNotMaterializeKeys(t *testing.T) {
	rt := newRuntime(&builtins.CallContext{}, &program{})
	rt.arrays["source"] = map[string]value{"a": numberValue(1), "b": numberValue(2)}
	rt.frames = append(rt.frames, callFrame{locals: map[string]*localVar{
		"items": {arrayAlias: &localVar{globalArrayName: "source"}},
	}})
	call := &callExpr{name: "length", args: []expr{&varExpr{name: "items"}}}

	var got value
	var err error
	allocs := testing.AllocsPerRun(100, func() {
		got, err = rt.evalLength(call)
	})

	require.NoError(t, err)
	require.Equal(t, float64(2), got.Number())
	require.Zero(t, allocs)
}
