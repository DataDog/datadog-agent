// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMonotonicDeltas(t *testing.T) {
	assert := assert.New(t)
	Clear()

	var deltas deltaCalculator
	t.Run("non-monotonic metric", func(t *testing.T) {
		state := deltas.GetState("")
		m := NewMetric("cache_size")

		// if metric is not flagged as monotonic we always return the current value
		m.Set(10)
		assert.Equal(int64(10), state.ValueFor(m))
		assert.Equal(int64(10), state.ValueFor(m))
		m.Set(5)
		assert.Equal(int64(5), state.ValueFor(m))
	})

	t.Run("monotonic metric", func(t *testing.T) {
		state := deltas.GetState("")
		m := NewMetric("requests_processed", OptMonotonic)

		m.Set(10)
		assert.Equal(int64(10), state.ValueFor(m))
		assert.Equal(int64(0), state.ValueFor(m))
		m.Set(15)
		assert.Equal(int64(5), state.ValueFor(m))
	})

	t.Run("one metric, multiple clients", func(t *testing.T) {
		stateA := deltas.GetState("clientA")
		stateB := deltas.GetState("clientB")
		m := NewMetric("connections_closed", OptMonotonic)

		m.Set(10)
		assert.Equal(int64(10), stateA.ValueFor(m))
		assert.Equal(int64(0), stateA.ValueFor(m))
		m.Set(15)
		assert.Equal(int64(5), stateA.ValueFor(m))
		assert.Equal(int64(15), stateB.ValueFor(m))
	})
}
