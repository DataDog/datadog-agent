// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !(pcap && cgo)

// Package rawpacket holds raw_packet related files
package rawpacket

import (
	"errors"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

// ProgOpts defines options
type ProgOpts struct {
	// MaxTailCalls maximun number of tail calls generated
	MaxTailCalls int
	// number of instructions
	MaxProgSize int
	// Number of nop instruction inserted in each program
	NopInstLen int
}

// DefaultProgOpts default options
var DefaultProgOpts ProgOpts

// BPFFilterToInsts compile a bpf filter expression
func BPFFilterToInsts(_ int, _ string, _ ProgOpts) (asm.Instructions, error) {
	return asm.Instructions{}, errors.New("not supported")
}

// FiltersToProgramSpecs returns list of program spec from raw packet filters definitions
func FiltersToProgramSpecs(_, _ int, _ []Filter, _ ProgOpts) ([]*ebpf.ProgramSpec, error) {
	return nil, errors.New("not supported")
}
