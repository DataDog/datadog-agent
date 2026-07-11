// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

// CWS is not supported on AIX; this file provides the minimum set of
// variable declarations and init stubs required by consts_common.go and
// external callers (probe, rules) so the package compiles on this platform.

package model

var (
	errorConstants = map[string]int{}
	// KernelCapabilityConstants list of kernel capabilities
	KernelCapabilityConstants = map[string]uint64{}
	addressFamilyConstants    = map[string]uint16{}

	// SignalConstants list of signals
	SignalConstants = map[string]int{}
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
func initSocketDomainConstants()               {}
func initSocketTypeConstants()                 {}
func initSocketFamilyConstants()               {}
func initSocketProtocolConstants()             {}
func initPrCtlOptionConstants()                {}
