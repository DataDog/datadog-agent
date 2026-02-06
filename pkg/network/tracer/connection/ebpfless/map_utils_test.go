// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package ebpfless

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteMapWithSizeLimit(t *testing.T) {
	m := map[string]int{}

	// not full: any write should work
	ok := WriteMapWithSizeLimit(m, "foo", 123, 1)
	require.True(t, ok)

	expectedFoo := map[string]int{
		"foo": 123,
	}
	require.Equal(t, expectedFoo, m)

	// full: shouldn't write a new key
	ok = WriteMapWithSizeLimit(m, "bar", 456, 1)
	require.False(t, ok)
	require.Equal(t, expectedFoo, m)

	// full: replacing key should still work
	ok = WriteMapWithSizeLimit(m, "foo", 789, 1)
	require.True(t, ok)
	require.Equal(t, map[string]int{
		"foo": 789,
	}, m)
}
