// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package protocols

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const roundMask uint64 = 1 << 10

func oldNSTimestampToFloat(ns uint64) float64 {
	var shift uint
	for ns > roundMask {
		ns = ns >> 1
		shift++
	}
	return float64(ns << shift)
}

func TestNSTimestampToFloat(t *testing.T) {
	ns := []uint64{
		uint64(1066789584153112 - 1066789583298779), // kernel boot time values
		uint64(0),
		uint64(1),
		uint64(1066789584153112),
		uint64(time.Hour * 24 * 3650), // 10 year
		uint64(time.Now().UnixNano()),
		uint64(0x000000000000ffff),
		uint64(1023),
		uint64(1024),
		uint64(1025),
		//^uint64(0), this can't be used here because float64 have only 52 bits of mantissa
		// and filter(float(uint64)) will difference due to roundup than float(filter(uint64))
		uint64(0x001fffffffffffff),
		^uint64(0x001fffffffffffff), // ~584 years
	}

	for _, n := range ns {
		require.Equal(t, oldNSTimestampToFloat(n), NSTimestampToFloat(n), "uint64 10 bits mantissa truncation failed "+fmt.Sprintf("%d 0x%x", n, n))
	}
}
