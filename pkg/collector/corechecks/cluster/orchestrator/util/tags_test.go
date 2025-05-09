// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImmutableTagsJoin(t *testing.T) {
	t.Run("slice with sufficient capacity to be mutated", func(t *testing.T) {
		original := make([]string, 0, 3)
		final := ImmutableTagsJoin(original, []string{"tag1", "tag2", "tag3"})
		assert.Empty(t, original)
		assert.Equal(t, []string{"", "", ""}, original[:cap(original)])
		assert.Equal(t, []string{"tag1", "tag2", "tag3"}, final)
	})
	t.Run("slice with insufficient capacity to be mutated", func(t *testing.T) {
		original := []string{"tag1", "tag2"}
		final := ImmutableTagsJoin(original, []string{"tag3", "tag4", "tag5"})
		assert.Equal(t, []string{"tag1", "tag2"}, original[:cap(original)])
		assert.Equal(t, []string{"tag1", "tag2", "tag3", "tag4", "tag5"}, final)
	})
	t.Run("no argument", func(t *testing.T) {
		final := ImmutableTagsJoin()
		assert.Nil(t, final)
	})
	t.Run("empty slices", func(t *testing.T) {
		final := ImmutableTagsJoin([]string{}, []string{})
		assert.Nil(t, final)
	})
}
