// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import "github.com/DataDog/datadog-agent/pkg/dyninst/ir"

// bitset is a data structure to store a dense set of booleans corresponding to
// indices. It is also used as the backing store for the per-expression status
// array (ExprStatusArray), where each expression occupies ExprStatusBits bits.
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

// getExprStatus reads the ir.ExprStatusBits-wide status for expression
// exprIdx from the packed bitset.
func (b bitset) getExprStatus(exprIdx int) ir.ExprStatus {
	bitOffset := exprIdx * ir.ExprStatusBits
	var v uint8
	for bit := range ir.ExprStatusBits {
		if b.get(bitOffset + bit) {
			v |= 1 << bit
		}
	}
	return ir.ExprStatus(v)
}
