// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package semantics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedRegistry(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)
	assert.NotEmpty(t, r.Version())
}

func TestGetAttributePrecedence(t *testing.T) {
	r, err := NewEmbeddedRegistry()
	require.NoError(t, err)

	t.Run("known concept returns tags", func(t *testing.T) {
		tags := r.GetAttributePrecedence(ConceptDBStatement)
		require.NotNil(t, tags)
		assert.Greater(t, len(tags), 0)
	})

	t.Run("unknown concept returns nil", func(t *testing.T) {
		tags := r.GetAttributePrecedence(Concept("unknown"))
		assert.Nil(t, tags)
	})
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	require.NotNil(t, r)
	assert.NotNil(t, r.GetAttributePrecedence(ConceptDBStatement))
}
