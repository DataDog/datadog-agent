// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"math"
	"math/bits"
)

const (
	exactCardinalityLimit = 65_536
	hllPrecision          = 14
	hllRegisterCount      = 1 << hllPrecision
)

// cardinalityCounter is exact for ordinary scenarios and switches to a fixed
// 16 KiB HyperLogLog sketch when cardinality is unusually high. This keeps a
// metadata statistic from defeating the bounded-memory replay pipeline.
type cardinalityCounter struct {
	exact     map[uint64]struct{}
	registers []uint8
}

func newCardinalityCounter() *cardinalityCounter {
	return &cardinalityCounter{exact: make(map[uint64]struct{})}
}

func (c *cardinalityCounter) Add(hash uint64) {
	if c.registers != nil {
		c.addHLL(hash)
		return
	}
	c.exact[hash] = struct{}{}
	if len(c.exact) <= exactCardinalityLimit {
		return
	}

	c.registers = make([]uint8, hllRegisterCount)
	for existing := range c.exact {
		c.addHLL(existing)
	}
	c.exact = nil
}

func (c *cardinalityCounter) Count() int {
	if c == nil {
		return 0
	}
	if c.registers == nil {
		return len(c.exact)
	}

	var (
		sum   float64
		zeros int
	)
	for _, register := range c.registers {
		sum += math.Ldexp(1, -int(register))
		if register == 0 {
			zeros++
		}
	}

	m := float64(len(c.registers))
	const alpha = 0.7213 / (1 + 1.079/hllRegisterCount)
	estimate := alpha * m * m / sum
	if estimate <= 2.5*m && zeros > 0 {
		estimate = m * math.Log(m/float64(zeros))
	}
	return int(math.Round(estimate))
}

func (c *cardinalityCounter) addHLL(hash uint64) {
	index := hash >> (64 - hllPrecision)
	remainder := hash << hllPrecision
	rank := bits.LeadingZeros64(remainder) + 1
	maxRank := 64 - hllPrecision + 1
	if rank > maxRank {
		rank = maxRank
	}
	if uint8(rank) > c.registers[index] {
		c.registers[index] = uint8(rank)
	}
}

func metricSeriesHash(name string, sortedTags []string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	hash := uint64(offset64)
	add := func(value string) {
		for i := 0; i < len(value); i++ {
			hash ^= uint64(value[i])
			hash *= prime64
		}
		hash ^= 0
		hash *= prime64
	}
	add(name)
	for _, tag := range sortedTags {
		add(tag)
	}
	return hash
}
