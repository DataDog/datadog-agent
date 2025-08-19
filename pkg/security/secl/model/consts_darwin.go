// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"golang.org/x/sys/unix"
)

var (
	errorConstants = map[string]int{}
	// KernelCapabilityConstants list of kernel capabilities
	KernelCapabilityConstants = map[string]uint64{}
	addressFamilyConstants    = map[string]uint16{}
)

var (
	// vmConstants on darwin are used by some dd-go tests, so we need them on darwin as well
	vmConstants = map[string]uint64{
		"VM_NONE":  0x0,
		"VM_READ":  0x1,
		"VM_WRITE": 0x2,
		"VM_EXEC":  0x4,
	}

	// SignalConstants on darwin are used by some dd-go tests, so we need them on darwin as well
	SignalConstants = map[string]int{
		"SIGKILL": int(unix.SIGKILL),
	}
)

func initVMConstants() {
	for k, v := range vmConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
	}
}

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
