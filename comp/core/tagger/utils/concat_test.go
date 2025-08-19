// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/require"
)

func TestEmptyArray(t *testing.T) {
	source := make([][]string, 0)
	result := ConcatenateTags(source...)
	require.Equal(t, []string{}, result)
}

func TestOneArray(t *testing.T) {
	source := make([][]string, 1)
	source[0] = []string{"one", "two"}
	result := ConcatenateTags(source...)
	require.Equal(t, []string{"one", "two"}, result)
}

func TestThreeArrays(t *testing.T) {
	source := make([][]string, 3)
	source[0] = []string{"one", "two"}
	source[1] = []string{}
	source[2] = []string{"4", "5", "6"}

	result := ConcatenateTags(source...)
	require.Equal(t, []string{"one", "two", "4", "5", "6"}, result)
}
