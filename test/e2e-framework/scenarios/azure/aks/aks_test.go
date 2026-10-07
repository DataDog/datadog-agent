// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeCountDefaultsToOne(t *testing.T) {
	params, err := NewParams()
	require.NoError(t, err)
	assert.Equal(t, 1, params.nodeCount)

	params, err = NewParams(WithNodeCount(2), WithKataNodePool())
	require.NoError(t, err)
	assert.Equal(t, 2, params.nodeCount)
	assert.True(t, params.kataNodePool)

	_, err = NewParams(WithNodeCount(0))
	assert.ErrorContains(t, err, "at least 1")
}
