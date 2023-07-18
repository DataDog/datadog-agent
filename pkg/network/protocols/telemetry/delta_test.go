// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeltas(t *testing.T) {
	assert := assert.New(t)
	Clear()

	var deltas deltaCalculator
	t.Run("gauge metric", func(t *testing.T) {
		// Delta calculator always returns the current value of a `Gauge` metric
		state := deltas.GetState("")
		m := NewGauge("cache_size")

		m.Set(10)
		assert.Equal(int64(10), state.ValueFor(m))
		assert.Equal(int64(10), state.ValueFor(m))
		m.Set(5)
		assert.Equal(int64(5), state.ValueFor(m))
	})

	t.Run("counter metric", func(t *testing.T) {
		state := deltas.GetState("")
		m := NewCounter("requests_processed")

		m.Add(10)
		assert.Equal(int64(10), state.ValueFor(m))
		assert.Equal(int64(0), state.ValueFor(m))
		m.Add(5)
		assert.Equal(int64(5), state.ValueFor(m))
	})

	t.Run("one metric, multiple clients", func(t *testing.T) {
		stateA := deltas.GetState("clientA")
		stateB := deltas.GetState("clientB")
		m := NewCounter("connections_closed")

		m.Add(10)
		assert.Equal(int64(10), stateA.ValueFor(m))
		assert.Equal(int64(0), stateA.ValueFor(m))
		m.Add(5)
		assert.Equal(int64(5), stateA.ValueFor(m))
		assert.Equal(int64(15), stateB.ValueFor(m))
	})
}
