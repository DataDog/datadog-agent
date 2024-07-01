// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package util contains common helpers used in the creation of the closed connection event handler
package util

import (
	"math"
	"os"

	manager "github.com/DataDog/ebpf-manager"
	cebpf "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/network"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
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

// ComputeDefaultFailedConnectionsRingBufferSize is the default buffer size of the ring buffer for closed connection events.
// Must be a power of 2 and a multiple of the page size
func ComputeDefaultFailedConnectionsRingBufferSize() int {
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

// ComputeDefaultFailedConnPerfBufferSize is the default buffer size of the perf buffer for closed connection events.
// Must be a multiple of the page size
func ComputeDefaultFailedConnPerfBufferSize() int {
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

// ConnStatsToTuple converts a ConnectionStats to a ConnTuple
func ConnStatsToTuple(c *network.ConnectionStats, tup *netebpf.ConnTuple) {
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
	if !c.Source.IsZero() {
		tup.Saddr_l, tup.Saddr_h = util.ToLowHigh(c.Source)
	}
	if !c.Dest.IsZero() {
		tup.Daddr_l, tup.Daddr_h = util.ToLowHigh(c.Dest)
	}
}
