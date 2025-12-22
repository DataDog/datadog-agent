// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

// bitset is a data structure to store a dense set of booleans corresponding to
// indices.
type bitset []byte

// reset initializes the bitset to the required size and clears it.
func (b *bitset) reset(n int) {
	requiredBytes := (n + 7) / 8
	if cap(*b) < requiredBytes {
		*b = make([]byte, requiredBytes)
	} else {
		*b = (*b)[:requiredBytes]
	}
	clear(*b)
}

// get checks if the given index is in range of the bitset and is set.
func (b bitset) get(index int) bool {
	idx, bit := index/8, index%8
	return idx < len(b) && b[idx]&(1<<byte(bit)) != 0
}

// set sets the given index to true if it is in the range that the bitset can
// represent.
func (b bitset) set(index int) {
	if idx, bit := index/8, index%8; idx < len(b) {
		b[idx] |= 1 << byte(bit)
	}
}
