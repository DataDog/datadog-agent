// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package model

var (
	errorConstants = map[string]int{}
	// KernelCapabilityConstants list of kernel capabilities
	KernelCapabilityConstants = map[string]uint64{}
	// SignalConstants list of signals
	SignalConstants        = map[string]int{}
	addressFamilyConstants = map[string]uint16{}
)

func initVMConstants()               {}
func initBPFCmdConstants()           {}
func initBPFHelperFuncConstants()    {}
func initBPFMapTypeConstants()       {}
func initBPFProgramTypeConstants()   {}
func initBPFAttachTypeConstants()    {}
func initPipeBufFlagConstants()      {}
func initOpenConstants()             {}
func initFileModeConstants()         {}
func initInodeModeConstants()        {}
func initUnlinkConstanst()           {}
func initKernelCapabilityConstants() {}
func initPtraceConstants()           {}
func initProtConstansts()            {}
func initMMapFlagsConstants()        {}
func initSignalConstants()           {}
func initBPFMapNamesConstants()      {}
