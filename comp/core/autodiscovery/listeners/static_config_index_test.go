// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStaticConfigIndex_AddRemoveHas(t *testing.T) {
	idx := NewStaticConfigIndex()

	assert.False(t, idx.Has("redis"))

	// First add transitions 0->1.
	assert.True(t, idx.Add("redis"))
	assert.True(t, idx.Has("redis"))

	// Second add for the same name does not transition.
	assert.False(t, idx.Add("redis"))
	assert.True(t, idx.Has("redis"))

	// First remove decrements but the name is still present.
	assert.False(t, idx.Remove("redis"))
	assert.True(t, idx.Has("redis"))

	// Second remove transitions 1->0.
	assert.True(t, idx.Remove("redis"))
	assert.False(t, idx.Has("redis"))

	// Remove on an absent name is a no-op.
	assert.False(t, idx.Remove("redis"))
	assert.False(t, idx.Remove("never-added"))
}

func TestStaticConfigIndex_NilSafe(t *testing.T) {
	var idx *StaticConfigIndex
	assert.False(t, idx.Has("anything"))
	assert.False(t, idx.Add("anything"))
	assert.False(t, idx.Remove("anything"))
}

func TestStaticConfigIndex_IndependentNames(t *testing.T) {
	idx := NewStaticConfigIndex()

	idx.Add("redis")
	idx.Add("postgres")

	assert.True(t, idx.Has("redis"))
	assert.True(t, idx.Has("postgres"))
	assert.False(t, idx.Has("mysql"))

	assert.True(t, idx.Remove("redis"))
	assert.False(t, idx.Has("redis"))
	assert.True(t, idx.Has("postgres"))
}
