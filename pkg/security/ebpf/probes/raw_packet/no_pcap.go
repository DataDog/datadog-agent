// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !(pcap && cgo)

// Package raw_packet holds raw_packet related files
package raw_packet

import (
	"errors"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

// RawPacketProgOpts defines options
type RawPacketProgOpts struct {
	// MaxTailCalls maximun number of tail calls generated
	MaxTailCalls int
	// number of instructions
	MaxProgSize int
	// Number of nop instruction inserted in each program
	NopInstLen int
}

// DefaultRawPacketProgOpts default options
var DefaultRawPacketProgOpts RawPacketProgOpts

// BPFFilterToInsts compile a bpf filter expression
func BPFFilterToInsts(_ int, _ string, _ RawPacketProgOpts) (asm.Instructions, error) {
	return asm.Instructions{}, errors.New("not supported")
}

// RawPacketTCFiltersToProgramSpecs returns list of program spec from raw packet filters definitions
func RawPacketTCFiltersToProgramSpecs(_, _ int, _ []RawPacketFilter, _ RawPacketProgOpts) ([]*ebpf.ProgramSpec, error) {
	return nil, errors.New("not supported")
}
