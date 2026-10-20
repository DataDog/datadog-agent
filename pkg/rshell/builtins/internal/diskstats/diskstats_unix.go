// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package diskstats

// subSat returns a - b, clamped to zero on underflow. Some kernel drivers
// (notably FUSE and CIFS variants) report f_bfree > f_blocks for
// transient states; clamping to zero keeps the listing sensible rather
// than wrapping a uint64 to ~16 EB.
func subSat(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

// mulSat returns a * b, clamped to MaxUint64 on overflow. Statfs(2)
// returns block / inode counts as uint64; multiplying by the block size
// can wrap if a buggy or malicious FUSE filesystem reports counts above
// MaxUint64 / bsize. Saturating keeps the displayed Total / Free / Used
// monotonic and prevents a single rogue mount from corrupting the
// --total accumulation.
func mulSat(a, b uint64) uint64 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > ^uint64(0)/b {
		return ^uint64(0)
	}
	return a * b
}
