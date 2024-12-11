// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"errors"
	"math"

	"golang.org/x/net/bpf"
)

var errInvalidSamplingRate = errors.New("sampling rate must be within (0, 1)")

// GenerateBPFSampler returns BPF assembly for a traffic sampler
func GenerateBPFSampler(samplingRate float64) ([]bpf.RawInstruction, error) {
	if samplingRate < 0 || samplingRate > 1 {
		return nil, errInvalidSamplingRate
	}

	cutoff := uint32(math.Pow(2, 32) * samplingRate)

	// Stolen from https://godoc.org/golang.org/x/net/bpf
	return bpf.Assemble([]bpf.Instruction{
		// Get a 32-bit random number from the Linux kernel.
		bpf.LoadExtension{Num: bpf.ExtRand},
		// If number is lower than cutoff, we capture  message
		bpf.JumpIf{Cond: bpf.JumpLessThan, Val: cutoff, SkipFalse: 1},
		// Capture.
		bpf.RetConstant{Val: 4096},
		// Ignore.
		bpf.RetConstant{Val: 0},
	})
}
