// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func BenchmarkAddPositive(b *testing.B) {
	m := NewCounter("foo")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Add(1)
	}
}

func BenchmarkAddZero(b *testing.B) {
	m := NewCounter("foo")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Add(0)
	}
}

func TestNewMetric(t *testing.T) {
	assert := assert.New(t)

	t.Run("different names", func(*testing.T) {
		Clear()
		m1 := NewCounter("m1")
		m2 := NewCounter("m2")

		assert.NotEqual(m1, m2)
	})

	t.Run("same name", func(*testing.T) {
		Clear()
		m1 := NewCounter("foo")
		m2 := NewCounter("foo")

		// ensure that if try to create the same metric
		// with the same name, we get back the existing
		// instance instead of creating a new one
		assert.Equal(m1, m2)
	})

	t.Run("same name and different tags", func(*testing.T) {
		Clear()
		m1 := NewCounter("foo", "name:bar", "cpu:0")
		m2 := NewCounter("foo", "name:bar", "cpu:1")

		assert.NotEqual(m1, m2)
	})

	t.Run("same name and tags", func(*testing.T) {
		Clear()
		// tag ordering doesn't matter
		m1 := NewCounter("foo", "name:bar", "cpu:0")
		m2 := NewCounter("foo", "cpu:0", "name:bar")

		assert.Equal(m1, m2)
	})
}

func TestMetricOperations(t *testing.T) {
	assert := assert.New(t)
	t.Run("counter metric", func(*testing.T) {
		Clear()

		m1 := NewCounter("m1")
		m1.Add(int64(5))
		assert.Equal(int64(5), m1.Get())
		assert.Equal(int64(5), m1.Get())

		m1.Add(int64(10))
		assert.Equal(int64(15), m1.Get())
		assert.Equal(int64(15), m1.Get())

		// Negative values are ignored as counters are always monotonic
		m1.Add(int64(-5))
		assert.Equal(int64(15), m1.Get())
		assert.Equal(int64(15), m1.Get())
	})
}
