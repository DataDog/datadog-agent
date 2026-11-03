// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package diskstats

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSubSat(t *testing.T) {
	cases := []struct{ a, b, want uint64 }{
		{0, 0, 0},
		{5, 3, 2},
		{3, 5, 0}, // underflow → 0
		{^uint64(0), 1, ^uint64(0) - 1},
		{1, ^uint64(0), 0}, // extreme underflow → 0
	}
	for _, c := range cases {
		assert.Equal(t, c.want, subSat(c.a, c.b), "a=%d b=%d", c.a, c.b)
	}
}

// TestMulSat — saturating multiply guards against buggy filesystems
// reporting block counts above MaxUint64/bsize. Without it, a single
// rogue mount could wrap to a tiny size and corrupt --total.
func TestMulSat(t *testing.T) {
	maxU := ^uint64(0)
	cases := []struct{ a, b, want uint64 }{
		{0, 0, 0},
		{0, 1234, 0},
		{1234, 0, 0},
		{2, 3, 6},
		{1 << 32, 1 << 30, 1 << 62},
		// Exact boundary: maxU/2 * 2 == maxU-1, no overflow.
		{maxU / 2, 2, maxU - 1},
		// Just over: (maxU/2 + 1) * 2 would wrap → saturates.
		{maxU/2 + 1, 2, maxU},
		// Extreme: maxU * 2 saturates.
		{maxU, 2, maxU},
		{maxU, maxU, maxU},
		// Realistic FUSE-rogue case: blocks reported as ~MaxUint64,
		// bsize=4096, would wrap to a tiny number without saturation.
		{maxU, 4096, maxU},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, mulSat(c.a, c.b), "a=%d b=%d", c.a, c.b)
	}
}
