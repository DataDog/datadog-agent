// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package teeconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapIsSubset(t *testing.T) {
	t.Run("empty base is subset of anything", func(t *testing.T) {
		assert.True(t, mapIsSubset(
			map[string]interface{}{},
			map[string]interface{}{"a": 1},
		))
	})

	t.Run("identical maps", func(t *testing.T) {
		assert.True(t, mapIsSubset(
			map[string]interface{}{"a": 1, "b": "two"},
			map[string]interface{}{"a": 1, "b": "two"},
		))
	})

	t.Run("base is strict subset of superset", func(t *testing.T) {
		assert.True(t, mapIsSubset(
			map[string]interface{}{"a": 1},
			map[string]interface{}{"a": 1, "b": 2, "c": 3},
		))
	})

	t.Run("base has key missing from superset", func(t *testing.T) {
		assert.False(t, mapIsSubset(
			map[string]interface{}{"a": 1, "missing": 2},
			map[string]interface{}{"a": 1},
		))
	})

	t.Run("same keys but different values", func(t *testing.T) {
		assert.False(t, mapIsSubset(
			map[string]interface{}{"a": 1},
			map[string]interface{}{"a": 99},
		))
	})

	t.Run("nested map values compared deeply", func(t *testing.T) {
		assert.True(t, mapIsSubset(
			map[string]interface{}{"nested": map[string]interface{}{"x": 1}},
			map[string]interface{}{"nested": map[string]interface{}{"x": 1}, "extra": true},
		))
		assert.False(t, mapIsSubset(
			map[string]interface{}{"nested": map[string]interface{}{"x": 1}},
			map[string]interface{}{"nested": map[string]interface{}{"x": 2}},
		))
	})
}
