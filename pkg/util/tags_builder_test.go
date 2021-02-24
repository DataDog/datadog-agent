// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewBuilder(t *testing.T) {
	tb := NewTagsBuilder()
	assert.NotNil(t, tb)
	assert.Equal(t, []string{}, tb.data)
}

func TestNewBuilderFromSlice(t *testing.T) {
	test := []string{"a", "b", "c"}
	tb := NewTagsBuilderFromSlice(test)
	assert.NotNil(t, tb)
	assert.Equal(t, test, tb.data)
}

func TestTagsBuilderAppend(t *testing.T) {
	tb := NewTagsBuilder()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)

	tb.Append("d")
	assert.Equal(t, []string{"a", "b", "c", "d"}, tb.data)
}

func TestTagsBuilderSortUniq(t *testing.T) {
	tb := NewTagsBuilder()

	tb.Append("c", "b", "b", "a")
	assert.Equal(t, []string{"c", "b", "b", "a"}, tb.data)

	tb.SortUniq()
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)
}

func TestTagsBuilderReset(t *testing.T) {
	tb := NewTagsBuilder()

	tb.Append("a", "b", "c")
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)

	tb.Reset()
	assert.Equal(t, []string{}, tb.data)
}

func TestTagsBuilderGet(t *testing.T) {
	tb := NewTagsBuilder()

	tb.Append("a", "b", "c")
	internalData := tb.Get()
	assert.Equal(t, []string{"a", "b", "c"}, internalData)

	// check that the internal buffer was indeed returned and not a copy
	internalData[0] = "test"
	assert.Equal(t, []string{"test", "b", "c"}, internalData)
	assert.Equal(t, []string{"test", "b", "c"}, tb.data)
}

func TestTagsBuilderCopy(t *testing.T) {
	tb := NewTagsBuilder()

	tb.Append("a", "b", "c")
	tagsCopy := tb.Copy()
	assert.Equal(t, []string{"a", "b", "c"}, tagsCopy)
	assert.NotSame(t, &tagsCopy, &tb.data)

	tagsCopy[0] = "test"
	assert.Equal(t, []string{"test", "b", "c"}, tagsCopy)
	assert.Equal(t, []string{"a", "b", "c"}, tb.data)
}
