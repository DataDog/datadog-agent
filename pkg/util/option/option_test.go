// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package option

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionConstructors(t *testing.T) {
	optional := New(42)
	v, ok := optional.Get()
	require.True(t, ok)
	require.Equal(t, 42, v)

	optional = None[int]()
	_, ok = optional.Get()
	require.False(t, ok)
}

func TestOptionSetReset(t *testing.T) {
	optional := New(0)
	optional.Set(42)
	v, ok := optional.Get()
	require.True(t, ok)
	require.Equal(t, 42, v)
	optional.Reset()
	_, ok = optional.Get()
	require.False(t, ok)
}

func TestMapOption(t *testing.T) {
	getLen := func(v string) int {
		return len(v)
	}

	optionalStr := New("hello")
	optionalInt := MapOption(optionalStr, getLen)

	v, ok := optionalInt.Get()
	require.True(t, ok)
	require.Equal(t, 5, v)

	optionalStr = None[string]()
	optionalInt = MapOption(optionalStr, getLen)

	_, ok = optionalInt.Get()
	require.False(t, ok)
}

func TestSetIfNone(t *testing.T) {
	optional := New(42)

	optional.SetIfNone(10)
	v, ok := optional.Get()
	require.Equal(t, 42, v)
	require.True(t, ok)

	optional.Reset()
	optional.SetIfNone(10)
	v, ok = optional.Get()
	require.Equal(t, 10, v)
	require.True(t, ok)
}

func TestSetOptionIfNone(t *testing.T) {
	optional := New(42)

	optional.SetOptionIfNone(New(10))
	v, ok := optional.Get()
	require.Equal(t, 42, v)
	require.True(t, ok)

	optional.Reset()
	optional.SetOptionIfNone(New(10))
	v, ok = optional.Get()
	require.Equal(t, 10, v)
	require.True(t, ok)
}
