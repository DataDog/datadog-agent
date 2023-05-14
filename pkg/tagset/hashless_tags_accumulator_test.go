// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewHashlessTagsAccumulator(t *testing.T) {
	tb := NewHashlessTagsAccumulator()
	assert.NotNil(t, tb)
	assert.Equal(t, []string{}, tb.data)
}

func TestNewHashlessTagsAccumulatorFromSlice(t *testing.T) {
	test := []string{"a", "b", "c"}
	tb := NewHashlessTagsAccumulatorFromSlice(test)
	assert.NotNil(t, tb)
	assert.Equal(t, test, tb.data)
}

func TestHashlessTagsAccumulatorAppend(t *testing.T) {
	tb := NewHashlessTagsAccumulator()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)

	tb.Append("d")
	assert.Equal(t, []string{"a", "b", "c", "d"}, tb.data)
}

func TestHashlessTagsAccumulatorReset(t *testing.T) {
	tb := NewHashlessTagsAccumulator()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)

	tb.Reset()
	assert.Equal(t, []string{}, tb.data)
}

func TestHashlessTagsAccumulatorGet(t *testing.T) {
	tb := NewHashlessTagsAccumulator()

	tb.Append("a", "b", "c")
	internalData := tb.Get()
	assert.Equal(t, []string{"a", "b", "c"}, internalData)

	// check that the internal buffer was indeed returned and not a copy
	internalData[0] = "test"
	assert.Equal(t, []string{"test", "b", "c"}, internalData)
	assert.Equal(t, []string{"test", "b", "c"}, tb.data)
}
