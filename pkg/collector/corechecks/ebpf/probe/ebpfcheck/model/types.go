// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model is the types for the eBPF check
package model

import (
	"time"

	"github.com/cilium/ebpf"
)

// EBPFStats contains the statistics from the ebpf check
type EBPFStats struct {
	Maps     []EBPFMapStats
	Programs []EBPFProgramStats
}

// EBPFMapStats are the basic statistics for ebpf maps
type EBPFMapStats struct {
	ID         uint32
	MaxEntries uint32
	Name       string
	Module     string
	RSS        uint64
	MaxSize    uint64
	Type       ebpf.MapType
	Entries    int64 // Allow negative values to indicate that the number of entries could not be calculated

	// used only for tests
	NumCPUs uint32
}

// EBPFProgramStats are the basic statistics for ebpf programs
type EBPFProgramStats struct {
	ID              uint32
	XlatedProgLen   uint32
	Name            string
	Module          string
	Tag             string
	RSS             uint64
	RunCount        uint64
	RecursionMisses uint64
	Runtime         time.Duration
	VerifiedInsns   uint32
	Type            ebpf.ProgramType
}
