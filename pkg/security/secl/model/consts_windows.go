// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

const (
	// SIGKILL id for the kill action
	SIGKILL = iota + 1
)

var (
	errorConstants         = map[string]int{}
	addressFamilyConstants = map[string]uint16{}

	// SignalConstants list of signals
	SignalConstants = map[string]int{
		"SIGKILL": SIGKILL,
	}
)

func initVMConstants()                         {}
func initBPFCmdConstants()                     {}
func initBPFHelperFuncConstants()              {}
func initBPFMapTypeConstants()                 {}
func initBPFProgramTypeConstants()             {}
func initBPFAttachTypeConstants()              {}
func initPipeBufFlagConstants()                {}
func initOpenConstants()                       {}
func initFileModeConstants()                   {}
func initInodeModeConstants()                  {}
func initUnlinkConstanst()                     {}
func initKernelCapabilityConstants()           {}
func initPtraceConstants()                     {}
func initProtConstansts()                      {}
func initMMapFlagsConstants()                  {}
func initSignalConstants()                     {}
func initBPFMapNamesConstants()                {}
func initAUIDConstants()                       {}
func initSysCtlActionConstants()               {}
func initSetSockOptLevelConstants()            {}
func initSetSockOptOptNameConstantsIP()        {}
func initSetSockOptOptNameConstantsSolSocket() {}
func initSetSockOptOptNameConstantsTCP()       {}
func initSetSockOptOptNameConstantsIPv6()      {}
func initRlimitConstants()                     {}
func initSocketTypeConstants()                 {}
func initSocketFamilyConstants()               {}
func initSocketProtocolConstants()             {}
