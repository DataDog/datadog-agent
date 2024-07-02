// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

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
		rk1 := aggregator.RollupKey(c1)
		rk2 := aggregator.RollupKey(c1)

		assert.Equal(t, c1, rk1)
		assert.Equal(t, c1, rk2)
	})

	t.Run("same key, flipped order", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)
		rk1 := aggregator.RollupKey(c1)

		c2 := types.NewConnectionKey(dstIP, srcIP, 80, 6000)
		rk2 := aggregator.RollupKey(c2)

		// Ensure the ordering of the key is preserved
		assert.Equal(t, c1, rk1)
		assert.Equal(t, c2, rk2)
	})

	t.Run("same IPs, but no matching ports", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)
		rk1 := aggregator.RollupKey(c1)

		c2 := types.NewConnectionKey(dstIP, srcIP, 7000, 53)
		rk2 := aggregator.RollupKey(c2)

		// In this case both keys are preserved, and there are no rollups
		assert.Equal(t, c1, rk1)
		assert.Equal(t, c2, rk2)
		assert.NotEqual(t, rk1, rk2)
	})

	t.Run("same IPs, different ephemeral ports", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)
		rk1 := aggregator.RollupKey(c1)

		c2 := types.NewConnectionKey(srcIP, dstIP, 6001, 80)
		rk2 := aggregator.RollupKey(c2)

		// Let's also try a different tuple order
		c3 := types.NewConnectionKey(dstIP, srcIP, 80, 6002)
		rk3 := aggregator.RollupKey(c3)

		c4 := types.NewConnectionKey(dstIP, srcIP, 80, 6003)
		rk4 := aggregator.RollupKey(c4)

		// Everything will be translated to essentially the same key, but tuple
		// order will be preserved.
		assert.Equal(t, c1, rk1)
		assert.Equal(t, c1, rk2)
		assert.Equal(t, c1, flipKey(rk3))
		assert.Equal(t, c1, flipKey(rk4))
	})

	t.Run("multiple server ports", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)
		rk1 := aggregator.RollupKey(c1)

		c2 := types.NewConnectionKey(srcIP, dstIP, 6001, 443)
		rk2 := aggregator.RollupKey(c2)

		c3 := types.NewConnectionKey(srcIP, dstIP, 6002, 80)
		rk3 := aggregator.RollupKey(c3)

		c4 := types.NewConnectionKey(srcIP, dstIP, 6003, 443)
		rk4 := aggregator.RollupKey(c4)

		// rk3 (*:80)  should be rolled up with rk1
		// rk4 (*:443) should be rolled up with rk2
		assert.Equal(t, rk1, rk3)
		assert.Equal(t, rk2, rk4)
	})

	t.Run("same server but different clients", func(t *testing.T) {
		var (
			clientA = util.AddressFromString("1.1.1.1")
			clientB = util.AddressFromString("2.2.2.2")
			server  = util.AddressFromString("3.3.3.3")
		)

		aggregator := NewConnectionAggregator()

		c1 := types.NewConnectionKey(clientA, server, 6000, 80)
		rk1 := aggregator.RollupKey(c1)

		c2 := types.NewConnectionKey(clientB, server, 6001, 80)
		rk2 := aggregator.RollupKey(c2)

		// Assert that this shouldn't trigger rollups
		assert.NotEqual(t, rk1, rk2)
		assert.Equal(t, c1, rk1)
		assert.Equal(t, c2, rk2)
	})
}

func TestClearEphemeralPort(t *testing.T) {
	t.Run("no state", func(t *testing.T) {
		aggregator := NewConnectionAggregator()
		srcIP := util.AddressFromString("1.1.1.1")
		dstIP := util.AddressFromString("2.2.2.2")

		c1 := types.NewConnectionKey(srcIP, dstIP, 6000, 80)

		// Nothing should happen in this case
		clearedC1, modified := aggregator.ClearEphemeralPort(c1)
		assert.False(t, modified)
		assert.Equal(t, c1, clearedC1)
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

		clearedC1, modified := aggregator.ClearEphemeralPort(c1)
		assert.True(t, modified)
		assert.Equal(t, expected, clearedC1)

		clearedC2, modified := aggregator.ClearEphemeralPort(c2)
		assert.True(t, modified)
		assert.Equal(t, expected, clearedC2)
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
		clearedC1, modified := aggregator.ClearEphemeralPort(c1)
		assert.True(t, modified)
		assert.Equal(t,
			types.NewConnectionKey(srcIP, dstIP, 0, 80),
			clearedC1,
		)

		clearedC2, modified := aggregator.ClearEphemeralPort(c2)
		assert.True(t, modified)
		assert.Equal(t,
			types.NewConnectionKey(dstIP, srcIP, 80, 0),
			clearedC2,
		)
	})
}

func TestNormalizeKey(t *testing.T) {
	t.Run("opposite keys, different IPs", func(t *testing.T) {
		var (
			ipA = util.AddressFromString("1.1.1.1")
			ipB = util.AddressFromString("2.2.2.2")
		)

		k1 := types.NewConnectionKey(ipA, ipB, 6001, 80)
		k2 := types.NewConnectionKey(ipB, ipA, 80, 6001)

		nk1, k1flipped := normalizeKey(k1)
		nk2, k2flipped := normalizeKey(k2)

		// k1 shouldn't change after normalization because srcIP < dstIP
		assert.False(t, k1flipped)
		assert.Equal(t, nk1, k1)

		// k2 should be flipped after normalization because srcIP > dstIP
		assert.True(t, k2flipped)
		assert.NotEqual(t, nk2, k2)

		// both keys should yield the same normalized result
		assert.Equal(t, nk1, nk2)
	})

	t.Run("opposite keys, but same IPs", func(t *testing.T) {
		localhost := util.AddressFromString("127.0.0.1")

		k1 := types.NewConnectionKey(localhost, localhost, 80, 6001)
		k2 := types.NewConnectionKey(localhost, localhost, 6001, 80)

		nk1, k1flipped := normalizeKey(k1)
		nk2, k2flipped := normalizeKey(k2)

		// k1 shouldn't change after normalization because the IPs are the same
		// and srcPort < dstPort
		assert.False(t, k1flipped)
		assert.Equal(t, nk1, k1)

		// k2 should be flipped after normalization because the IPs are the same
		// and srcPort > dstPort
		assert.True(t, k2flipped)
		assert.NotEqual(t, nk2, k2)

		// both keys should yield the same normalized result
		assert.Equal(t, nk1, nk2)
	})

	t.Run("when srcIP < dstIP values are preserved", func(t *testing.T) {
		ipA := util.AddressFromString("1.1.1.1")
		ipB := util.AddressFromString("2.2.2.2")

		key := types.NewConnectionKey(ipA, ipB, 60000, 80)
		normalizedKey, flipped := normalizeKey(key)
		assert.False(t, flipped)
		assert.Equal(t, key, normalizedKey)
	})

	t.Run("when srcIP < dstIP values are flipped", func(t *testing.T) {
		ipA := util.AddressFromString("1.1.1.1")
		ipB := util.AddressFromString("2.2.2.2")

		key := types.NewConnectionKey(ipB, ipA, 60000, 80)
		normalizedKey, flipped := normalizeKey(key)
		assert.True(t, flipped)

		assert.Equal(t, key.SrcIPHigh, normalizedKey.DstIPHigh)
		assert.Equal(t, key.SrcIPLow, normalizedKey.DstIPLow)
		assert.Equal(t, key.DstIPHigh, normalizedKey.SrcIPHigh)
		assert.Equal(t, key.DstIPLow, normalizedKey.SrcIPLow)
		assert.Equal(t, key.SrcPort, normalizedKey.DstPort)
	})
}
