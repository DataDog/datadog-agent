// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTagList(t *testing.T) {
	list := NewTagList()
	require.NotNil(t, list)
	require.NotNil(t, list.lowCardTags)
	require.NotNil(t, list.highCardTags)
	low, high := list.Compute()
	require.NotNil(t, low)
	require.Empty(t, low)
	require.NotNil(t, high)
	require.Empty(t, high)
}

func TestAddLow(t *testing.T) {
	list := NewTagList()
	list.AddLow("foo", "bar")
	list.AddLow("faa", "baz")
	require.Empty(t, list.highCardTags)
	require.Len(t, list.lowCardTags, 2)
	require.Equal(t, "bar", list.lowCardTags["foo"])
	require.Equal(t, "baz", list.lowCardTags["faa"])
}

func TestAddHigh(t *testing.T) {
	list := NewTagList()
	list.AddHigh("foo", "bar")
	list.AddHigh("faa", "baz")
	require.Empty(t, 0, list.lowCardTags)
	require.Len(t, list.highCardTags, 2)
	require.Equal(t, "bar", list.highCardTags["foo"])
	require.Equal(t, "baz", list.highCardTags["faa"])
}

func TestCompute(t *testing.T) {
	list := NewTagList()
	list.AddHigh("foo", "bar")
	list.AddLow("faa", "baz")
	list.AddLow("low", "yes")

	low, high := list.Compute()
	require.Len(t, low, 2)
	require.Contains(t, low, "faa:baz")
	require.Contains(t, low, "low:yes")
	require.Len(t, high, 1)
	require.Contains(t, high, "foo:bar")
}
