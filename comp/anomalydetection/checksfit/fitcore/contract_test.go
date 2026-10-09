// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

//go:build linux || (darwin && cgo)

package fitcore

import (
	"strings"
	"testing"
)

// The golden bytes must match the Rust template exactly; both implementations
// derive them from the same shared contract.
func TestContractBytesMatchRustGolden(t *testing.T) {
	descriptor := ProtocolDescriptor{
		ID:           array8("METRIC01"),
		Version:      2,
		MessageTypes: []uint32{1, 2},
	}
	// Captured from the Rust metric example consumer's mismatch diagnostic:
	// setup version 1, protocol version 2, layout version 3, producer role,
	// "METRIC01", "MQUEUE03".
	const golden = "000000010000000200000003014d455452494330314d51554555453033"
	if got := hexBytes(contractBytes(1, descriptor)); got != golden {
		t.Fatalf("producer contract = %s, want %s", got, golden)
	}
	// The consumer tuple differs only in the role byte.
	const goldenConsumer = "000000010000000200000003024d455452494330314d51554555453033"
	if got := hexBytes(contractBytes(2, descriptor)); got != goldenConsumer {
		t.Fatalf("consumer contract = %s, want %s", got, goldenConsumer)
	}
	if err := checkContract(contractBytes(1, descriptor), 1, descriptor); err != nil {
		t.Fatalf("matching contract rejected: %v", err)
	}
	err := checkContract(contractBytes(2, descriptor), 1, descriptor)
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("role mismatch accepted: %v", err)
	}
}

func TestDescriptorRegistryValidation(t *testing.T) {
	for _, kinds := range [][]uint32{{}, {0}, {1, 1}} {
		descriptor := ProtocolDescriptor{ID: array8("TEST0001"), Version: 1, MessageTypes: kinds}
		if err := descriptor.validate(); err == nil {
			t.Fatalf("invalid registry accepted: %v", kinds)
		}
	}
	descriptor := ProtocolDescriptor{ID: array8("TEST0001"), Version: 1, MessageTypes: []uint32{1, 42}}
	if err := descriptor.validate(); err != nil {
		t.Fatalf("valid registry rejected: %v", err)
	}
	if !descriptor.supports(42) {
		t.Fatal("descriptor does not support type 42")
	}
}
