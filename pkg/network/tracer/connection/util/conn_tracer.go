// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package util contains common helpers used for kernel network tracing
package util

import (
	"math"
	"os"

	manager "github.com/DataDog/ebpf-manager"
	cebpf "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// toPowerOf2 converts a number to its nearest power of 2
func toPowerOf2(x int) int {
	log2 := math.Log2(float64(x))
	return int(math.Pow(2, math.Round(log2)))
}

// ComputeDefaultClosedConnRingBufferSize is the default buffer size of the ring buffer for closed connection events.
// Must be a power of 2 and a multiple of the page size
func ComputeDefaultClosedConnRingBufferSize() int {
	numCPUs, err := cebpf.PossibleCPU()
	if err != nil {
		numCPUs = 1
	}
	return 8 * toPowerOf2(numCPUs) * os.Getpagesize()
}

// ComputeDefaultClosedConnPerfBufferSize is the default buffer size of the perf buffer for closed connection events.
// Must be a multiple of the page size
func ComputeDefaultClosedConnPerfBufferSize() int {
	return 8 * os.Getpagesize()
}

// AddBoolConst modifies the options to include a constant editor for a boolean value
func AddBoolConst(options *manager.Options, name string, flag bool) {
	val := uint64(1)
	if !flag {
		val = uint64(0)
	}

	options.ConstantEditors = append(options.ConstantEditors,
		manager.ConstantEditor{
			Name:  name,
			Value: val,
		},
	)
}

// ConnTupleToEBPFTuple converts a ConnectionTuple to an eBPF ConnTuple
func ConnTupleToEBPFTuple(c *network.ConnectionTuple, tup *netebpf.ConnTuple) {
	tup.Sport = c.SPort
	tup.Dport = c.DPort
	tup.Netns = c.NetNS
	tup.Pid = c.Pid
	if c.Family == network.AFINET {
		tup.SetFamily(netebpf.IPv4)
	} else {
		tup.SetFamily(netebpf.IPv6)
	}
	if c.Type == network.TCP {
		tup.SetType(netebpf.TCP)
	} else {
		tup.SetType(netebpf.UDP)
	}
	if c.Source.IsValid() {
		tup.Saddr_l, tup.Saddr_h = util.ToLowHigh(c.Source)
	}
	if c.Dest.IsValid() {
		tup.Daddr_l, tup.Daddr_h = util.ToLowHigh(c.Dest)
	}
}

// tlsDiagMinimumKernel is the lowest kernel on which the TLS-misclassification diagnostics are
// enabled. The threshold is deliberately conservative.
//
// The functional boundary is 5.2, where both BPF_MAXINSNS and the verifier's complexity limit jump
// from 4,096 / 131,072 to 1,000,000. Below that, older verifiers do far less path pruning, and the
// diagnostics' extra branches were enough to stop socket__classifier_entry loading in the prebuilt
// object on 4.18 (KMT: ubuntu_18.04, amazon_4.14 — TLS classification silently disabled, while the
// runtime-compiled tracer passed because host-specific compilation drops the version-gated branches
// that make the prebuilt object more complex).
//
// 6.0 is used rather than 5.2 because these diagnostics are a temporary investigation aid only ever
// deployed to 6.x staging hosts, so there is no reason to carry any risk on older kernels.
var tlsDiagMinimumKernel = kernel.VersionCode(6, 0, 0)

// TLSDiagnosticsSupported reports whether the TLS-misclassification diagnostics should be active.
//
// Gating matters for more than tidiness: the constant is patched into the program before load, so
// when it is false the verifier sees a known-zero scalar and prunes the diagnostic branches
// entirely, returning the classifier programs to their baseline complexity. Note it does NOT reduce
// program size — the dead instructions still count against BPF_MAXINSNS — so this only helps with
// complexity, which is what the pre-5.2 failure was.
func TLSDiagnosticsSupported() bool {
	kv, err := kernel.HostVersion()
	if err != nil {
		// Fail closed: without a version we cannot rule out an old verifier.
		return false
	}
	return kv >= tlsDiagMinimumKernel
}
