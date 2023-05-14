// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMetric(t *testing.T) {
	assert := assert.New(t)

	t.Run("different names", func(t *testing.T) {
		Clear()
		m1 := NewMetric("m1")
		m2 := NewMetric("m2")

		assert.NotEqual(m1, m2)
	})

	t.Run("same name", func(t *testing.T) {
		Clear()
		m1 := NewMetric("foo")
		m2 := NewMetric("foo")

		// ensure that if try to create the same metric
		// with the same name, we get back the existing
		// instance instead of creating a new one
		assert.Equal(m1, m2)
	})

	t.Run("same name and different tags", func(t *testing.T) {
		Clear()
		m1 := NewMetric("foo", "name:bar", "cpu:0")
		m2 := NewMetric("foo", "name:bar", "cpu:1")

		assert.NotEqual(m1, m2)
	})

	t.Run("same name and tags", func(t *testing.T) {
		Clear()
		// tag ordering doesn't matter
		m1 := NewMetric("foo", "name:bar", "cpu:0")
		m2 := NewMetric("foo", "cpu:0", "name:bar")

		assert.Equal(m1, m2)
	})
}

func TestMetricOperations(t *testing.T) {
	assert := assert.New(t)

	t.Run("regular (non-monotonic) metric", func(t *testing.T) {
		Clear()

		m1 := NewMetric("m1")
		m1.Add(int64(5))
		assert.Equal(int64(5), m1.Get())

		m1.Add(int64(5))
		assert.Equal(int64(10), m1.Get())

		v := m1.Swap(int64(0))
		assert.Equal(int64(10), v)
		assert.Equal(int64(0), m1.Get())

		m1.Set(20)
		assert.Equal(int64(20), m1.Get())
	})

	t.Run("monotonic metric", func(t *testing.T) {
		Clear()

		m1 := NewMetric("m1", OptMonotonic)
		m1.Add(int64(5))
		assert.Equal(int64(5), m1.Get())
		assert.Equal(int64(5), m1.Delta())
		assert.Equal(int64(0), m1.Delta())
		assert.Equal(int64(5), m1.Get())

		m1.Add(int64(10))
		assert.Equal(int64(15), m1.Get())
		assert.Equal(int64(10), m1.Delta())
		assert.Equal(int64(0), m1.Delta())
		assert.Equal(int64(15), m1.Get())
	})
}
