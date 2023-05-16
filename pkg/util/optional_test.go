// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionConstructors(t *testing.T) {
	optional := NewOptional(42)
	v, ok := optional.Get()
	require.True(t, ok)
	require.Equal(t, 42, v)

	optional = NewNoneOptional[int]()
	_, ok = optional.Get()
	require.False(t, ok)
}

func TestOptionSetReset(t *testing.T) {
	optional := NewOptional(0)
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

	optionalStr := NewOptional("hello")
	optionalInt := MapOptional(optionalStr, getLen)

	v, ok := optionalInt.Get()
	require.True(t, ok)
	require.Equal(t, 5, v)

	optionalStr = NewNoneOptional[string]()
	optionalInt = MapOptional(optionalStr, getLen)

	_, ok = optionalInt.Get()
	require.False(t, ok)
}
