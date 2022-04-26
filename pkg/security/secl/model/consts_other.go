// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux
// +build !linux

package model

var (
	errorConstants     = map[string]int{}
	openFlagsConstants = map[string]int{}
	chmodModeConstants = map[string]int{}
	// KernelCapabilityConstants list of kernel capabilities
	KernelCapabilityConstants = map[string]uint64{}
	unlinkFlagsConstants      = map[string]int{}
	ptraceConstants           = map[string]uint32{}
	ptraceArchConstants       = map[string]uint32{}
	protConstants             = map[string]int{}
	mmapFlagConstants         = map[string]int{}
	mmapFlagArchConstants     = map[string]int{}
	signalConstants           = map[string]int{}
	addressFamilyuConstants   = map[string]int{}
)
