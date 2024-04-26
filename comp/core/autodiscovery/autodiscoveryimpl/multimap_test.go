// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package autodiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMultimap(t *testing.T) {
	mm := newMultimap()

	mm.insert("foo", "f")
	mm.insert("foo", "o")
	mm.insert("foo", "o")
	mm.insert("bar", "b")
	mm.insert("bar", "a")
	mm.insert("bar", "r")

	require.Equal(t, []string{"f", "o", "o"}, mm.get("foo"))
	require.Equal(t, []string{"b", "a", "r"}, mm.get("bar"))
	require.Equal(t, []string{}, mm.get("bing"))

	mm.remove("foo", "o") // only removes one of the two o's
	mm.remove("bar", "a")

	require.Equal(t, []string{"f", "o"}, mm.get("foo"))
	require.Equal(t, []string{"b", "r"}, mm.get("bar"))

	mm.remove("foo", "o")
	mm.remove("foo", "f")

	require.Equal(t, []string{}, mm.get("foo"))
	require.Equal(t, []string{"b", "r"}, mm.get("bar"))
}
