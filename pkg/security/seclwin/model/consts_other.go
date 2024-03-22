// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

var (
	errorConstants     = map[string]int{}
	openFlagsConstants = map[string]int{}
	fileModeConstants  = map[string]int{}
	inodeModeConstants = map[string]int{}
	// KernelCapabilityConstants list of kernel capabilities
	KernelCapabilityConstants = map[string]uint64{}
	unlinkFlagsConstants      = map[string]int{}
	ptraceConstants           = map[string]uint32{}
	ptraceArchConstants       = map[string]uint32{}
	protConstants             = map[string]int{}
	mmapFlagConstants         = map[string]uint64{}
	mmapFlagArchConstants     = map[string]uint64{}
	// SignalConstants list of signals
	SignalConstants        = map[string]int{}
	addressFamilyConstants = map[string]uint16{}
)
