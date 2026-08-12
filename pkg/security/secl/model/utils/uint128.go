// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utility related to the model
package utils

import (
	"fmt"
	"math/big"
	"strconv"
)

// TraceID is a 128-bit identifier for a trace.
type TraceID struct {
	Lo uint64
	Hi uint64
}

func (t TraceID) bigInt() *big.Int {
	hi := big.NewInt(0)
	hi.SetUint64(t.Hi)
	hi.Lsh(hi, 64)

	lo := big.NewInt(0)
	lo.SetUint64(t.Lo)

	return hi.Add(hi, lo)
}

func (t TraceID) String() string {
	return t.bigInt().String()
}

// HexString returns the trace ID as lowercase hex.
//
// When Hi == 0 (the high half was not collected, e.g. dd-trace-go pprof
// labels expose only the lower 64 bits), Lo is emitted unpadded so the
// result is byte-exact for backend pattern searches against the uint64
// lower half.
//
// When Hi != 0 the full 128-bit ID was collected and the result must
// match the canonical 16-byte form (e.g. APM's 32-char display): Hi is
// emitted unpadded but Lo is zero-padded to 16 hex chars so leading
// zeros in Lo — which are part of the real 128-bit ID — are preserved.
func (t TraceID) HexString() string {
	if t.Hi == 0 {
		return strconv.FormatUint(t.Lo, 16)
	}
	return fmt.Sprintf("%x%016x", t.Hi, t.Lo)
}
