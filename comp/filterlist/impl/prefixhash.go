// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filterlistimpl

import (
	"encoding/binary"
	"math/bits"
	"unsafe"
)

// hasByte returns a bitmask with the high bit set in each byte position that equals b.
func hasByte(x uint64, b byte) uint64 {
	// broadcast b to all lanes
	m := uint64(b) * 0x0101010101010101
	y := x ^ m
	// classic hasZeroByte trick on y
	return (y - 0x0101010101010101) & (^y) & 0x8080808080808080
}

// firstMatchByteIndex returns [0..7] for the first matching byte in mask (mask from hasByte).
func firstMatchByteIndex(mask uint64) int {
	// mask has high bits set per matching byte; trailing zeros /8 gives index
	return bits.TrailingZeros64(mask) >> 3
}

// A very fast 64-bit hash mixer (non-crypt). Good for bucketing/sharding.
func mix64(h uint64) uint64 {
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return h
}

// HashUntilColon hashes s up to the first ':' and returns (hash, pos).
// pos is the index of ':' or len(s) if absent.
func HashUntilColon(s string) (uint64, int) {
	n := len(s)
	if n == 0 {
		return 0, 0
	}

	p := unsafe.StringData(s)
	i := 0
	var h uint64 = 0x9e3779b97f4a7c15 // seed

	// process 8 bytes at a time
	for i+8 <= n {
		w := binary.LittleEndian.Uint64(unsafe.Slice((*byte)(unsafe.Add(unsafe.Pointer(p), i)), 8))
		if mask := hasByte(w, ':'); mask != 0 {
			off := firstMatchByteIndex(mask)
			// hash bytes before ':'
			// hash full bytes before match: [i, i+off)
			// Mix as little-endian; handle partial in tail loop below.
			for j := 0; j < off; j++ {
				h = h*0x9e3779b185ebca87 + uint64(unsafe.Slice(p, n)[i+j])
			}
			return mix64(h), i + off
		}

		// no ':' in this word, fold whole word quickly
		h ^= w * 0x9e3779b185ebca87
		h = bits.RotateLeft64(h, 27) * 0x94d049bb133111eb
		i += 8
	}

	// tail (also finds ':' if in the last <8 bytes)
	for i < n {
		c := *(*byte)(unsafe.Add(unsafe.Pointer(p), i))
		if c == ':' {
			return mix64(h), i
		}
		h = h*0x9e3779b185ebca87 + uint64(c)
		i++
	}
	return mix64(h), n
}
