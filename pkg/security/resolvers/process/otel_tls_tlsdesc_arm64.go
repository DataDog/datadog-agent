// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && arm64

package process

import (
	"encoding/binary"
)

const otelTLSDescInstSz = 4

// staticResolverBody is the entire static TLSDESC resolver on aarch64:
//
//	f9400400  ldr x0, [x0, #8]
//	d65f03c0  ret
//
// Ported from go.opentelemetry.io/ebpf-profiler/tls: glibc, musl, bionic and
// uClibc-ng all write this body in hand-written assembly.
var otelTLSDescStaticResolverBody = [...]uint32{0xf9400400, 0xd65f03c0}

// maxLandingPads bounds the hints tolerated ahead of the body. glibc and
// bionic emit one when built for BTI; musl and uClibc-ng none.
const otelTLSDescMaxLandingPads = 2

const otelTLSDescResolverCodeSize = (len(otelTLSDescStaticResolverBody) + otelTLSDescMaxLandingPads) * otelTLSDescInstSz

// HINT encoding space: bits 11:5 hold the immediate. Nothing in it writes x0.
const (
	otelTLSDescHintMask = 0xfffff01f
	otelTLSDescHintBits = 0xd503201f
)

func otelTLSDescIsHint(w uint32) bool { return w&otelTLSDescHintMask == otelTLSDescHintBits }

// tlsdescReturnsArg reports whether the TLSDESC resolver at addr hands back
// the descriptor's argument word unchanged, which is what makes that argument
// a TP-relative offset on aarch64.
func tlsdescReturnsArg(mem otelMemReader, addr uint64) (bool, error) {
	var code [otelTLSDescResolverCodeSize]byte
	if err := mem.Read(addr, code[:]); err != nil {
		return false, err
	}
	word := func(i int) uint32 { return binary.LittleEndian.Uint32(code[i*otelTLSDescInstSz:]) }

	i := 0
	for i < otelTLSDescMaxLandingPads && otelTLSDescIsHint(word(i)) {
		i++
	}
	for j, want := range otelTLSDescStaticResolverBody {
		if word(i+j) != want {
			return false, nil
		}
	}
	return true, nil
}
