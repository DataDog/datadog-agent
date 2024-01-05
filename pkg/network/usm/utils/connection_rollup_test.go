// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestRollupKey(t *testing.T) {
	t.Run("same key", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)
		t1 := aggregator.RollupKey(c1)
		t2 := aggregator.RollupKey(c1)

		assert.Equal(t, c1, t1)
		assert.Equal(t, c1, t2)
	})

	t.Run("same key, flipped order", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)
		t1 := aggregator.RollupKey(c1)

		c2 := types.NewConnectionKey(dstIP, srcIP, 80, 6000)
		t2 := aggregator.RollupKey(c2)

		assert.Equal(t, c1, t1)
		assert.Equal(t, c1, t2)
	})

	t.Run("same IPs, but no matching ports", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)
		t1 := aggregator.RollupKey(c1)

		c2 := types.NewConnectionKey(dstIP, srcIP, 7000, 53)
		t2 := aggregator.RollupKey(c2)

		// In this case both keys are preserved, which wouldn't trigger a rollup
		assert.Equal(t, c1, t1)
		assert.Equal(t, c2, t2)
		assert.NotEqual(t, c1, c2)
	})

	t.Run("same IPs, different ephemeral ports", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)
		t1 := aggregator.RollupKey(c1)

		c2 := types.NewConnectionKey(srcIP, dstIP, 6001, 80)
		t2 := aggregator.RollupKey(c2)

		// Let's also try a different tuple order
		c3 := types.NewConnectionKey(dstIP, srcIP, 80, 6002)
		t3 := aggregator.RollupKey(c3)

		// c1, c2 and c3 should all translate to c1
		assert.Equal(t, c1, t1)
		assert.Equal(t, c1, t2)
		assert.Equal(t, c1, t3)
	})
}

func TestClearEphemeralPort(t *testing.T) {
	t.Run("no state", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)

		// Nothing should happen in this case
		assert.Equal(t, c1, aggregator.ClearEphemeralPort(c1))
	})

	t.Run("base case", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6001, 80)
		_ = aggregator.RollupKey(c1)
		c2 := types.NewConnectionKey(srcIP, dstIP, 6002, 80)
		_ = aggregator.RollupKey(c2)

		// In this case both c1 and c2 should generated the same redacted key
		// with the ephemeral port side set to 0
		expected := types.NewConnectionKey(srcIP, dstIP, 0, 80)

		assert.Equal(t, expected, aggregator.ClearEphemeralPort(c1))
		assert.Equal(t, expected, aggregator.ClearEphemeralPort(c2))
	})

	t.Run("flipped tuples", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6001, 80)
		_ = aggregator.RollupKey(c1)
		c2 := types.NewConnectionKey(dstIP, srcIP, 80, 6002)
		_ = aggregator.RollupKey(c2)

		// The order of the tuples should be preserved, but 6001/6002 ports
		// should still be correctly cleared
		assert.Equal(t,
			types.NewConnectionKey(srcIP, dstIP, 0, 80),
			aggregator.ClearEphemeralPort(c1),
		)
		assert.Equal(t,
			types.NewConnectionKey(dstIP, srcIP, 80, 0),
			aggregator.ClearEphemeralPort(c2),
		)
	})
}
