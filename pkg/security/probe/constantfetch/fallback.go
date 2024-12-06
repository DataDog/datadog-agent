// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package constantfetch holds constantfetch related files
package constantfetch

import (
	"errors"
	"os"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

// FallbackConstantFetcher is a constant fetcher that uses the old fallback
// heuristics to fetch constants
type FallbackConstantFetcher struct {
	kernelVersion *kernel.Version
	res           map[string]uint64
}

// NewFallbackConstantFetcher returns a new FallbackConstantFetcher
func NewFallbackConstantFetcher(kv *kernel.Version) *FallbackConstantFetcher {
	return &FallbackConstantFetcher{
		kernelVersion: kv,
		res:           make(map[string]uint64),
	}
}

func (f *FallbackConstantFetcher) String() string {
	return "fallback"
}

func (f *FallbackConstantFetcher) appendRequest(id string) {
	var value = ErrorSentinel
	switch id {
	case SizeOfInode:
		value = getSizeOfStructInode(f.kernelVersion)
	case OffsetNameSuperBlockStructSFlags:
		value = getSuperBlockFlagsOffset(f.kernelVersion)
	case OffsetNameSuperBlockStructSMagic:
		value = getSuperBlockMagicOffset(f.kernelVersion)
	case OffsetNameSignalStructStructTTY:
		value = getSignalTTYOffset(f.kernelVersion)
	case OffsetNameTTYStructStructName:
		value = getTTYNameOffset(f.kernelVersion)
	case OffsetNameCredStructUID:
		value = getCredsUIDOffset(f.kernelVersion)
	case OffsetNameCredStructCapInheritable:
		value = getCredCapInheritableOffset(f.kernelVersion)
	case OffsetNameBPFMapStructID:
		value = getBpfMapIDOffset(f.kernelVersion)
	case OffsetNameBPFMapStructName:
		value = getBpfMapNameOffset(f.kernelVersion)
	case OffsetNameBPFMapStructMapType:
		value = getBpfMapTypeOffset(f.kernelVersion)
	case OffsetNameBPFProgStructAux:
		value = getBpfProgAuxOffset(f.kernelVersion)
	case OffsetNameBPFProgStructTag:
		value = getBpfProgTagOffset(f.kernelVersion)
	case OffsetNameBPFProgStructType:
		value = getBpfProgTypeOffset(f.kernelVersion)
	case OffsetNameBPFProgStructExpectedAttachType:
		value = getBpfProgAttachTypeOffset(f.kernelVersion)
	case OffsetNameBPFProgAuxStructID:
		value = getBpfProgAuxIDOffset(f.kernelVersion)
	case OffsetNameBPFProgAuxStructName:
		value = getBpfProgAuxNameOffset(f.kernelVersion)
	case OffsetNamePIDStructLevel:
		value = getPIDLevelOffset(f.kernelVersion)
	case OffsetNamePIDStructNumbers:
		value = getPIDNumbersOffset(f.kernelVersion)
	case SizeOfUPID:
		value = getSizeOfUpid(f.kernelVersion)
	case OffsetNameTaskStructPID:
		value = getTaskStructPIDOffset(f.kernelVersion)
	case OffsetNameTaskStructPIDLink:
		value = getTaskStructPIDLinkOffset(f.kernelVersion)
	case OffsetNamePIDLinkStructPID:
		value = getPIDLinkPIDOffset(f.kernelVersion)
	case OffsetNameDentryStructDSB:
		value = getDentrySuperBlockOffset(f.kernelVersion)
	case OffsetNamePipeInodeInfoStructBufs:
		value = getPipeInodeInfoBufsOffset(f.kernelVersion)
	case OffsetNamePipeInodeInfoStructNrbufs:
		value = getPipeInodeInfoStructNrbufs(f.kernelVersion)
	case OffsetNamePipeInodeInfoStructCurbuf:
		value = getPipeInodeInfoStructCurbuf(f.kernelVersion)
	case OffsetNamePipeInodeInfoStructBuffers:
		value = getPipeInodeInfoStructBuffers(f.kernelVersion)
	case OffsetNamePipeInodeInfoStructHead:
		value = getPipeInodeInfoStructHead(f.kernelVersion)
	case OffsetNamePipeInodeInfoStructRingsize:
		value = getPipeInodeInfoStructRingsize(f.kernelVersion)
	case OffsetNameNetDeviceStructIfIndex:
		value = getNetDeviceIfindexOffset(f.kernelVersion)
	case OffsetNameNetDeviceStructName:
		value = getNetDeviceNameOffset(f.kernelVersion)
	case OffsetNameNetStructNS:
		value = getNetNSOffset(f.kernelVersion)
	case OffsetNameNetStructProcInum:
		value = getNetProcINumOffset(f.kernelVersion)
	case OffsetNameSockCommonStructSKCNet:
		value = getSockCommonSKCNetOffset(f.kernelVersion)
	case OffsetNameSocketStructSK:
		value = getSocketSockOffset(f.kernelVersion)
	case OffsetNameNFConnStructCTNet:
		value = getNFConnCTNetOffset(f.kernelVersion)
	case OffsetNameSockCommonStructSKCFamily:
		value = getSockCommonSKCFamilyOffset(f.kernelVersion)
	case OffsetNameFlowI4StructSADDR:
		value = getFlowi4SAddrOffset(f.kernelVersion)
	case OffsetNameFlowI6StructSADDR:
		value = getFlowi6SAddrOffset(f.kernelVersion)
	case OffsetNameFlowI4StructULI:
		value = getFlowi4ULIOffset(f.kernelVersion)
	case OffsetNameFlowI6StructULI:
		value = getFlowi6ULIOffset(f.kernelVersion)
	case OffsetNameLinuxBinprmStructFile:
		value = getBinPrmFileFieldOffset(f.kernelVersion)
	case OffsetNameIoKiocbStructCtx:
		value = getIoKcbCtxOffset(f.kernelVersion)
	case OffsetNameLinuxBinprmP:
		value = getLinuxBinPrmPOffset(f.kernelVersion)
	case OffsetNameLinuxBinprmArgc:
		value = getLinuxBinPrmArgcOffset(f.kernelVersion)
	case OffsetNameLinuxBinprmEnvc:
		value = getLinuxBinPrmEnvcOffset(f.kernelVersion)
	case OffsetNameVMAreaStructFlags:
		value = getVMAreaStructFlagsOffset(f.kernelVersion)
	case OffsetNameKernelCloneArgsExitSignal:
		value = getKernelCloneArgsExitSignalOffset(f.kernelVersion)
	case OffsetNameFileFinode:
		value = getFileFinodeOffset(f.kernelVersion)
	case OffsetNameFileFpath:
		value = getFileFpathOffset(f.kernelVersion)
	case OffsetNameMountMntID:
		value = getMountIDOffset(f.kernelVersion)
	case OffsetNameRenameStructOldDentry:
		value = getRenameStructOldDentryOffset(f.kernelVersion)
	case OffsetNameRenameStructNewDentry:
		value = getRenameStructNewDentryOffset(f.kernelVersion)
	}
	f.res[id] = value
}

// AppendSizeofRequest appends a sizeof request
func (f *FallbackConstantFetcher) AppendSizeofRequest(id, _, _ string) {
	f.appendRequest(id)
}

// AppendOffsetofRequest appends an offset request
func (f *FallbackConstantFetcher) AppendOffsetofRequest(id, _, _, _ string) {
	f.appendRequest(id)
}

// FinishAndGetResults returns the results
func (f *FallbackConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	return f.res, nil
}

func getSizeOfStructInode(kv *kernel.Version) uint64 {
	sizeOf := uint64(600)

	// see https://ubuntu.com/security/CVE-2019-10638
	increaseSizeAbiMinVersion := map[string]int{
		"generic":      99,
		"generic-lpae": 99,
		"lowlatency":   99,
		"gke":          1058,
		"gcp":          1093,
		"aws":          1066,
		"azure":        1082,
	}

	switch {
	case kv.IsRH7Kernel():
		sizeOf = 584
	case kv.IsRH8Kernel() || kv.IsRH9Kernel():
		sizeOf = 648
	case kv.IsSuse12Kernel():
		if kv.IsInRangeCloseOpen(kernel.Kernel4_12, kernel.Kernel4_13) && kv.Code.Patch() >= 14 {
			sizeOf = 592
		} else {
			sizeOf = 560
		}
	case kv.IsSuse15Kernel():
		sizeOf = 592
	case kv.IsOracleUEKKernel():
		sizeOf = 632
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel4_20):
		sizeOf = 712
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		sizeOf = 704
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		sizeOf = 704
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		if kv.Code.Patch() > 250 {
			sizeOf = 592
		} else {
			sizeOf = 584
		}
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		if kv.Code.Patch() > 100 {
			sizeOf = 592
		} else {
			sizeOf = 584
		}
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_15, kernel.Kernel5_16):
		sizeOf = 616
	case kv.IsAmazonLinux2023Kernel() && kv.IsInRangeCloseOpen(kernel.Kernel6_1, kernel.Kernel6_2):
		sizeOf = 624
	case kv.IsInRangeCloseOpen(kernel.Kernel4_15, kernel.Kernel4_16):
		if ubuntuAbiVersionCheck(kv, increaseSizeAbiMinVersion) {
			sizeOf = 608
		} else {
			sizeOf = 600
		}
	case kv.Code != 0 && kv.Code < kernel.Kernel4_16:
		sizeOf = 608
	case kv.IsInRangeCloseOpen(kernel.Kernel5_0, kernel.Kernel5_1):
		sizeOf = 584
	case kv.IsInRangeCloseOpen(kernel.Kernel5_13, kernel.Kernel5_15):
		sizeOf = 592
	case kv.Code >= kernel.Kernel5_15:
		sizeOf = 632
	}

	return sizeOf
}

func getSuperBlockFlagsOffset(_ *kernel.Version) uint64 {
	return uint64(80)
}

func getSuperBlockMagicOffset(kv *kernel.Version) uint64 {
	offset := uint64(96)

	if kv.IsRH7Kernel() {
		offset = 88
	}

	return offset
}

// Depending on the value CONFIG_NO_HZ_FULL, a field can be added before the `tty` field.
// See https://elixir.bootlin.com/linux/v5.18/source/include/linux/sched/signal.h#L164
func getNoHzOffset() uint64 {
	if _, err := os.Stat("/sys/devices/system/cpu/nohz_full"); errors.Is(err, os.ErrNotExist) {
		return 0
	}
	return 8
}

func getSignalTTYOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.IsRH7Kernel():
		return 416
	case kv.IsRH8Kernel():
		return 392
	case kv.IsRH9Kernel():
		return 424
	case kv.IsSuse12Kernel():
		return 376
	case kv.IsSuse15Kernel():
		return 408
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel4_20):
		return 416
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		return 416
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		return 416
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_14, kernel.Kernel4_15):
		return 368
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		return 400 + getNoHzOffset()
	case kv.IsAmazonLinux2023Kernel() && kv.IsInRangeCloseOpen(kernel.Kernel6_1, kernel.Kernel6_2):
		return 408
	case kv.IsUbuntuKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_15, kernel.Kernel4_19):
		return 368 + getNoHzOffset()
	case kv.IsUbuntuKernel() && kv.Code < kernel.Kernel5_19:
		return 400 + getNoHzOffset()
	case kv.IsUbuntuKernel() && kv.Code >= kernel.Kernel5_19:
		return 408 + getNoHzOffset()
	case kv.Code >= kernel.Kernel5_16:
		return 416
	}

	return 400 + getNoHzOffset()
}

func getTTYNameOffset(kv *kernel.Version) uint64 {
	nameOffset := uint64(368)

	switch {
	case kv.IsRH7Kernel():
		nameOffset = 312
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel4_20):
		nameOffset = 552
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		nameOffset = 552
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		nameOffset = 544
	case kv.IsInRangeCloseOpen(kernel.Kernel4_13, kernel.Kernel5_8):
		nameOffset = 368
	case kv.IsInRangeCloseOpen(kernel.Kernel5_8, kernel.Kernel5_9) && runtime.GOARCH == "arm64":
		nameOffset = 368
	case kv.IsInRangeCloseOpen(kernel.Kernel5_8, kernel.Kernel5_14):
		nameOffset = 360
	case kv.Code >= kernel.Kernel5_14:
		nameOffset = 352
	}

	return nameOffset
}

func getCredsUIDOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.IsCOSKernel():
		return 20
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5) && kv.Code.Patch() > 250:
		return 8
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11) && kv.Code.Patch() > 200:
		return 8
	case kv.IsDebianKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel4_20) && kv.Code.Patch() > 250:
		return 8
	case kv.IsDebianKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11) && kv.Code.Patch() > 200:
		return 8
	case kv.IsDebianKernel() && kv.IsInRangeCloseOpen(kernel.Kernel6_1, kernel.Kernel6_2) && kv.Code.Patch() > 70:
		return 8
	default:
		return 4
	}
}

func getCredCapInheritableOffset(kv *kernel.Version) uint64 {
	return getCredsUIDOffset(kv) + 36
}

func getBpfMapIDOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.IsInRangeCloseOpen(kernel.Kernel5_15, kernel.Kernel5_16):
		return 52
	case kv.IsInRangeCloseOpen(kernel.Kernel5_16, kernel.Kernel5_19) || kv.IsRH9Kernel():
		return 60
	case kv.IsInRangeCloseOpen(kernel.Kernel5_19, kernel.Kernel6_2):
		return 68
	case kv.Code >= kernel.Kernel6_2:
		return 52
	default:
		return 48
	}
}

func getBpfMapNameOffset(kv *kernel.Version) uint64 {
	nameOffset := uint64(168)

	switch {
	case kv.IsRH7Kernel():
		nameOffset = 112
	case kv.IsRH8Kernel():
		nameOffset = 80
	case kv.IsRH9Kernel():
		nameOffset = 96
	case kv.IsSuse15Kernel():
		nameOffset = 88
	case kv.IsSuse12Kernel():
		nameOffset = 176

	case kv.IsInRangeCloseOpen(kernel.Kernel4_15, kernel.Kernel4_18):
		nameOffset = 112
	case kv.IsInRangeCloseOpen(kernel.Kernel4_18, kernel.Kernel5_1):
		nameOffset = 176
	case kv.IsInRangeCloseOpen(kernel.Kernel5_1, kernel.Kernel5_3):
		nameOffset = 200
	case kv.IsInRangeCloseOpen(kernel.Kernel5_3, kernel.Kernel5_5):
		if kv.IsOracleUEKKernel() {
			nameOffset = 200
		} else {
			nameOffset = 168
		}
	case kv.IsInRangeCloseOpen(kernel.Kernel5_5, kernel.Kernel5_11):
		nameOffset = 88
	case kv.IsInRangeCloseOpen(kernel.Kernel5_11, kernel.Kernel5_13):
		nameOffset = 80
	case kv.IsInRangeCloseOpen(kernel.Kernel5_13, kernel.Kernel5_15):
		nameOffset = 80
	case kv.IsInRangeCloseOpen(kernel.Kernel5_15, kernel.Kernel5_16):
		nameOffset = 88
	case kv.IsInRangeCloseOpen(kernel.Kernel5_16, kernel.Kernel5_19):
		nameOffset = 96
	case kv.IsInRangeCloseOpen(kernel.Kernel5_19, kernel.Kernel6_2):
		nameOffset = 104
	case kv.Code >= kernel.Kernel6_2:
		nameOffset = 96
	case kv.Code != 0 && kv.Code < kernel.Kernel4_15:
		return ErrorSentinel
	}

	return nameOffset
}

func getBpfMapTypeOffset(_ *kernel.Version) uint64 {
	return uint64(24)
}

func getBpfProgAuxOffset(kv *kernel.Version) uint64 {
	auxOffset := uint64(32)

	switch {
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_14, kernel.Kernel4_15):
		auxOffset = 24
	case kv.IsInRangeCloseOpen(kernel.Kernel4_15, kernel.Kernel4_16):
		auxOffset = 24
	case kv.Code >= kernel.Kernel5_13:
		auxOffset = 56
	}

	return auxOffset
}

func getBpfProgTagOffset(kv *kernel.Version) uint64 {
	progTagOffset := uint64(20)
	switch {
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_14, kernel.Kernel4_15):
		progTagOffset = 16
	case kv.IsInRangeCloseOpen(kernel.Kernel4_15, kernel.Kernel4_16):
		progTagOffset = 16
	}

	return progTagOffset
}

func getBpfProgTypeOffset(_ *kernel.Version) uint64 {
	return uint64(4)
}

func getBpfProgAttachTypeOffset(_ *kernel.Version) uint64 {
	return uint64(8)
}

func getBpfProgAuxIDOffset(kv *kernel.Version) uint64 {
	idOffset := uint64(24)

	switch {
	case kv.IsRH7Kernel():
		idOffset = 8
	case kv.IsRH8Kernel():
		idOffset = 32
	case kv.IsSuse15Kernel():
		idOffset = 28
	case kv.IsSuse12Kernel():
		idOffset = 16
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_14, kernel.Kernel4_15):
		idOffset = 16

	case kv.IsInRangeCloseOpen(kernel.Kernel4_15, kernel.Kernel5_0):
		idOffset = 16
	case kv.IsInRangeCloseOpen(kernel.Kernel5_0, kernel.Kernel5_4):
		idOffset = 20
	case kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_8):
		idOffset = 24
	case kv.IsInRangeCloseOpen(kernel.Kernel5_8, kernel.Kernel5_13):
		idOffset = 28
	case kv.Code != 0 && kv.Code >= kernel.Kernel5_13:
		idOffset = 32
	}

	return idOffset
}

func getBpfProgAuxNameOffset(kv *kernel.Version) uint64 {
	nameOffset := uint64(176)

	switch {
	case kv.IsRH7Kernel():
		nameOffset = 144
	case kv.IsRH8Kernel():
		nameOffset = 520
	case kv.IsRH9Kernel():
		nameOffset = 544
	case kv.IsSuse15Kernel():
		if kv.IsInRangeCloseOpen(kernel.Kernel5_3, kernel.Kernel5_4) {
			nameOffset = 424
		} else {
			nameOffset = 256
		}
	case kv.IsSuse12Kernel():
		nameOffset = 160
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		nameOffset = 544

	case kv.IsInRangeCloseOpen(kernel.Kernel4_15, kernel.Kernel4_18):
		nameOffset = 128
	case kv.IsInRangeCloseOpen(kernel.Kernel4_18, kernel.Kernel4_19):
		nameOffset = 152
	case kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel5_0):
		nameOffset = 160
	case kv.IsInRangeCloseOpen(kernel.Kernel5_0, kernel.Kernel5_8):
		nameOffset = 176
	case kv.IsInRangeCloseOpen(kernel.Kernel5_8, kernel.Kernel5_10):
		nameOffset = 416
	case kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		nameOffset = 496
	case kv.IsInRangeCloseOpen(kernel.Kernel5_11, kernel.Kernel5_13):
		nameOffset = 504
	case kv.IsInRangeCloseOpen(kernel.Kernel5_13, kernel.Kernel5_16):
		nameOffset = 528
	case kv.IsInRangeCloseOpen(kernel.Kernel5_16, kernel.Kernel5_17):
		nameOffset = 544
	case kv.IsInRangeCloseOpen(kernel.Kernel5_17, kernel.Kernel6_1):
		nameOffset = 528
	case kv.Code != 0 && kv.Code >= kernel.Kernel6_1:
		nameOffset = 912
	}

	return nameOffset
}

func getPIDLevelOffset(_ *kernel.Version) uint64 {
	return uint64(4)
}

func getPIDNumbersOffset(kv *kernel.Version) uint64 {
	pidNumbersOffset := uint64(48)

	switch {
	case kv.IsRH7Kernel():
		pidNumbersOffset = 48
	case kv.IsRH8Kernel():
		pidNumbersOffset = 56
	case kv.IsSuse12Kernel():
		pidNumbersOffset = 48
	case kv.IsSuse15Kernel():
		pidNumbersOffset = 80
	case kv.IsDebianKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel4_20):
		pidNumbersOffset = 56
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel4_20):
		pidNumbersOffset = 56
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		pidNumbersOffset = 96
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		pidNumbersOffset = 128

	case kv.IsInRangeCloseOpen(kernel.Kernel4_15, kernel.Kernel5_0):
		pidNumbersOffset = 48
	case kv.IsInRangeCloseOpen(kernel.Kernel5_0, kernel.Kernel5_1):
		pidNumbersOffset = 56
	case kv.IsInRangeCloseOpen(kernel.Kernel5_1, kernel.Kernel5_3):
		pidNumbersOffset = 48
	case kv.IsInRangeCloseOpen(kernel.Kernel5_0, kernel.Kernel5_3):
		pidNumbersOffset = 56
	case kv.IsInRangeCloseOpen(kernel.Kernel5_3, kernel.Kernel5_7):
		pidNumbersOffset = 80
	case kv.Code != 0 && kv.Code >= kernel.Kernel5_7:
		pidNumbersOffset = 96
	}
	return pidNumbersOffset
}

func getSizeOfUpid(kv *kernel.Version) uint64 {
	sizeOfUpid := uint64(16)

	switch {
	case kv.IsRH7Kernel():
		sizeOfUpid = 32
	case kv.IsRH8Kernel():
		sizeOfUpid = 16
	case kv.IsSuse12Kernel():
		if kv.IsInRangeCloseOpen(kernel.Kernel4_12, kernel.Kernel4_13) && kv.Code.Patch() >= 14 {
			sizeOfUpid = 32
		} else {
			sizeOfUpid = 16
		}
	case kv.IsSuse15Kernel():
		if kv.IsInRangeCloseOpen(kernel.Kernel5_3, kernel.Kernel5_4) {
			sizeOfUpid = 16
		} else {
			sizeOfUpid = 32
		}
	case kv.IsAmazonLinuxKernel() && kv.Code != 0 && kv.Code < kernel.Kernel4_15:
		sizeOfUpid = 32
	}
	return sizeOfUpid
}

func getDentrySuperBlockOffset(kv *kernel.Version) uint64 {
	offset := uint64(104)

	switch {
	case kv.IsCOSKernel():
		offset = 128
	}

	return offset
}

func getPipeInodeInfoBufsOffset(kv *kernel.Version) uint64 {
	offset := uint64(120)

	switch {
	case kv.IsRH7Kernel():
		offset = 128
	case kv.IsRH8Kernel():
		offset = 120
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		offset = 152
	case kv.IsDebianKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11) && kv.Code.Patch() > 46:
		offset = 152
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel4_20):
		fallthrough
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		offset = 160
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		offset = 208

	case kv.IsInRangeCloseOpen(kernel.Kernel4_13, kernel.Kernel5_6):
		offset = 120
	case kv.IsInRangeCloseOpen(kernel.Kernel5_6, kernel.Kernel5_8) ||
		kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		offset = 144
	case kv.Code != 0 && kv.Code >= kernel.Kernel5_8:
		offset = 152
	}

	return offset
}

func getPipeInodeInfoStructNrbufs(kv *kernel.Version) uint64 {
	offset := ErrorSentinel
	if kv.HaveLegacyPipeInodeInfoStruct() {
		offset = 56
		switch {
		case kv.IsDebianKernel() && strings.Contains(kv.UnameRelease, "-rt-"):
			offset = 104
		case kv.Code < kernel.Kernel4_10:
			offset = 64
		}
	}
	return offset
}

func getPipeInodeInfoStructCurbuf(kv *kernel.Version) uint64 {
	offset := ErrorSentinel
	if kv.HaveLegacyPipeInodeInfoStruct() {
		offset = 60
		switch {
		case kv.IsDebianKernel() && strings.Contains(kv.UnameRelease, "-rt-"):
			offset = 108
		case kv.Code < kernel.Kernel4_10:
			offset = 68
		}
	}
	return offset
}

func getPipeInodeInfoStructBuffers(kv *kernel.Version) uint64 {
	offset := ErrorSentinel
	if kv.HaveLegacyPipeInodeInfoStruct() {
		offset = 64
		switch {
		case kv.IsDebianKernel() && strings.Contains(kv.UnameRelease, "-rt-"):
			offset = 112
		case kv.Code < kernel.Kernel4_10:
			offset = 72
		}
	}
	return offset
}

func getPipeInodeInfoStructHead(kv *kernel.Version) uint64 {
	offset := ErrorSentinel
	if !kv.HaveLegacyPipeInodeInfoStruct() {
		offset = 80
		if kv.IsDebianKernel() && strings.Contains(kv.UnameRelease, "-rt-") {
			offset = 168
		}
	}
	return offset
}

func getPipeInodeInfoStructRingsize(kv *kernel.Version) uint64 {
	offset := ErrorSentinel
	if !kv.HaveLegacyPipeInodeInfoStruct() {
		offset = 92
		if kv.IsDebianKernel() && strings.Contains(kv.UnameRelease, "-rt-") {
			offset = 180
		}
	}
	return offset
}

func getNetDeviceIfindexOffset(kv *kernel.Version) uint64 {
	offset := uint64(260)

	switch {
	case kv.IsRH7Kernel():
		offset = 192
	case kv.IsRH8Kernel():
		offset = 264
	case kv.IsSuse12Kernel():
		offset = 264
	case kv.IsSuse15Kernel():
		offset = 256

	case kv.IsInRangeCloseOpen(kernel.Kernel4_14, kernel.Kernel5_8):
		offset = 264
	case kv.IsInRangeCloseOpen(kernel.Kernel5_8, kernel.Kernel5_12):
		offset = 256
	case kv.IsInRangeCloseOpen(kernel.Kernel5_12, kernel.Kernel5_17):
		offset = 208
	case kv.IsUbuntuKernel() && kv.IsInRangeCloseOpen(kernel.Kernel6_5, kernel.Kernel6_6):
		offset = 224
	case kv.Code >= kernel.Kernel5_17:
		offset = 216
	}

	return offset
}

func getNetNSOffset(kv *kernel.Version) uint64 {
	// see https://ubuntu.com/security/CVE-2019-10638
	hashMixAbiMinVersion := map[string]int{
		"generic":      60,
		"generic-lpae": 60,
		"lowlatency":   60,
		"oracle":       1022,
		"gke":          1041,
		"gcp":          1042,
		"aws":          1047,
		"azure":        1018,
	}

	switch {
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel4_20):
		return 176
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		fallthrough
	case kv.IsCOSKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_10, kernel.Kernel5_11):
		return 192
	case kv.IsInRangeCloseOpen(kernel.Kernel4_15, kernel.Kernel4_16) && ubuntuAbiVersionCheck(kv, hashMixAbiMinVersion):
		fallthrough
	// Commit 355b98553789b646ed97ad801a619ff898471b92 introduces a hashmix field for security
	// purposes. This commit was cherry-picked in stable releases 4.9.168, 4.14.111, 4.19.34 and 5.0.7
	// and is part of master since 5.1
	case kv.IsRH8Kernel():
		fallthrough
	case kv.IsSuse12Kernel():
		fallthrough
	case (kv.IsInRangeCloseOpen(kernel.Kernel4_9, kernel.Kernel4_10) && kv.Code.Patch() >= 168) ||
		(kv.IsInRangeCloseOpen(kernel.Kernel4_14, kernel.Kernel4_15) && kv.Code.Patch() >= 111) ||
		kv.Code >= kernel.Kernel5_1:
		return 120
	default:
		return 112
	}
}

func getNetProcINumOffset(_ *kernel.Version) uint64 {
	return uint64(72)
}

func getSockCommonSKCNetOffset(_ *kernel.Version) uint64 {
	return uint64(48)
}

func getSocketSockOffset(kv *kernel.Version) uint64 {
	offset := uint64(32)

	switch {
	case kv.IsRH7Kernel():
		offset = 32
	case kv.IsRH8Kernel():
		offset = 32
	case kv.IsSuse12Kernel():
		offset = 32
	case kv.IsSuse15Kernel():
		offset = 24

	case kv.Code >= kernel.Kernel5_3:
		offset = 24
	}

	return offset
}

func getNFConnCTNetOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.IsCOSKernel():
		return 168
	case kv.IsRH7Kernel():
		return 240
	case kv.IsRH9Kernel():
		fallthrough
	case kv.Code >= kernel.Kernel5_19:
		return 136
	default:
		return 144
	}
}

func getSockCommonSKCFamilyOffset(_ *kernel.Version) uint64 {
	return 16
}

func getFlowi4SAddrOffset(kv *kernel.Version) uint64 {
	offset := uint64(40)

	switch {
	case kv.IsRH7Kernel():
		offset = 20
	case kv.IsRH8Kernel():
		offset = 56
	case kv.IsOracleUEKKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		offset = 56
	case kv.IsDebianKernel() && kv.IsInRangeCloseOpen(kernel.Kernel6_1, kernel.Kernel6_2) && kv.Code.Patch() > 70:
		offset = 40

	case kv.IsInRangeCloseOpen(kernel.Kernel5_0, kernel.Kernel5_1):
		offset = 32
	case kv.IsInRangeCloseOpen(kernel.Kernel5_1, kernel.Kernel5_18):
		offset = 40
	case kv.Code >= kernel.Kernel5_18:
		offset = 48
	}

	return offset
}

func getFlowi4ULIOffset(kv *kernel.Version) uint64 {
	return getFlowi4SAddrOffset(kv) + 8
}

func getFlowi6SAddrOffset(kv *kernel.Version) uint64 {
	return getFlowi4ULIOffset(kv) + 8
}

func getFlowi6ULIOffset(kv *kernel.Version) uint64 {
	return getFlowi6SAddrOffset(kv) + 20
}

func ubuntuAbiVersionCheck(kv *kernel.Version, minAbiPerFlavor map[string]int) bool {
	ukv := kv.UbuntuKernelVersion()
	if ukv == nil {
		return false
	}

	minAbi, present := minAbiPerFlavor[ukv.Flavor]
	if !present {
		return false
	}

	return ukv.Abi >= minAbi
}

// getBinPrmFileFieldOffset returns the offset of the file field in the linux_binprm struct depending on the kernel version that the system probe is running on. Only used if runtime compilation, btf co-re, btfhub, offset-guesser all fail to yield an offset value.
func getBinPrmFileFieldOffset(kv *kernel.Version) uint64 {
	if kv.IsRH8Kernel() {
		return 296
	}

	if kv.IsRH7Kernel() || kv.Code < kernel.Kernel5_0 {
		return 168
	}

	if kv.Code >= kernel.Kernel5_0 && kv.Code < kernel.Kernel5_2 {
		// `unsigned long argmin` is introduced in v5.0-rc1
		return 176
	}

	if kv.Code >= kernel.Kernel5_2 && kv.Code < kernel.Kernel5_8 {
		// `char buf[BINPRM_BUF_SIZE]` is removed in v5.2-rc1
		return 48
	}

	// `struct file *executable` and `struct file *interpreter` are introduced in v5.8-rc1
	return 64
}

func getIoKcbCtxOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.IsOracleUEKKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		return 80
	case kv.IsUbuntuKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5):
		return 96
	case kv.IsAmazonLinuxKernel() && kv.IsInRangeCloseOpen(kernel.Kernel5_4, kernel.Kernel5_5) && kv.Code.Patch() > 250:
		return 96
	case kv.Code >= kernel.Kernel5_16:
		return 88
	default:
		return 80
	}
}

func getLinuxBinPrmPOffset(kv *kernel.Version) uint64 {
	offset := uint64(152)

	switch {
	case kv.Code >= kernel.Kernel5_2:
		offset = 24
	case kv.IsRH8Kernel():
		fallthrough
	case kv.IsAmazonLinuxKernel() && kv.Code == kernel.Kernel4_14 &&
		(kv.Code.Patch() == uint8(146) || kv.Code.Patch() == uint8(152) || kv.Code.Patch() == uint8(154) ||
			kv.Code.Patch() == uint8(158) || kv.Code.Patch() == uint8(200) || kv.Code.Patch() == uint8(203)):
		offset = 280
	}

	return offset
}

func getLinuxBinPrmArgcOffset(kv *kernel.Version) uint64 {
	offset := uint64(192)

	switch {
	case kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel5_0):
		offset = 192
	case kv.IsInRangeCloseOpen(kernel.Kernel5_0, kernel.Kernel5_2):
		offset = 200
	case kv.IsInRangeCloseOpen(kernel.Kernel5_2, kernel.Kernel5_8):
		offset = 72
	case kv.Code >= kernel.Kernel5_8:
		offset = 88
	case kv.IsRH8Kernel():
		fallthrough
	case kv.IsAmazonLinuxKernel() && kv.Code == kernel.Kernel4_14 &&
		(kv.Code.Patch() == uint8(146) || kv.Code.Patch() == uint8(152) || kv.Code.Patch() == uint8(154) ||
			kv.Code.Patch() == uint8(158) || kv.Code.Patch() == uint8(200) || kv.Code.Patch() == uint8(203)):
		offset = 320
	}

	return offset
}

func getLinuxBinPrmEnvcOffset(kv *kernel.Version) uint64 {
	offset := uint64(196)

	switch {
	case kv.IsInRangeCloseOpen(kernel.Kernel4_19, kernel.Kernel5_0):
		offset = 196
	case kv.IsInRangeCloseOpen(kernel.Kernel5_0, kernel.Kernel5_2):
		offset = 204
	case kv.IsInRangeCloseOpen(kernel.Kernel5_2, kernel.Kernel5_8):
		offset = 76
	case kv.Code >= kernel.Kernel5_8:
		offset = 92
	case kv.IsRH8Kernel():
		fallthrough
	case kv.IsAmazonLinuxKernel() && kv.Code == kernel.Kernel4_14 &&
		(kv.Code.Patch() == uint8(146) || kv.Code.Patch() == uint8(152) || kv.Code.Patch() == uint8(154) ||
			kv.Code.Patch() == uint8(158) || kv.Code.Patch() == uint8(200) || kv.Code.Patch() == uint8(203)):
		offset = 324
	}

	return offset
}

func getVMAreaStructFlagsOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.Code >= kernel.Kernel6_1:
		return 32
	}
	return 80
}

func getTaskStructPIDOffset(_ *kernel.Version) uint64 {
	// do not use fallback for offsets inside task_struct
	return ErrorSentinel
}

func getTaskStructPIDLinkOffset(_ *kernel.Version) uint64 {
	// do not use fallback for offsets inside task_struct
	return ErrorSentinel
}

func getPIDLinkPIDOffset(kv *kernel.Version) uint64 {
	offset := ErrorSentinel
	if kv.HavePIDLinkStruct() {
		offset = uint64(16)
	}
	return offset
}

func getKernelCloneArgsExitSignalOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.IsUbuntuKernel() && kv.IsInRangeCloseOpen(kernel.Kernel6_5, kernel.Kernel6_6):
		return 40
	default:
		return 32
	}
}

func getFileFinodeOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.IsUbuntuKernel() && kv.IsInRangeCloseOpen(kernel.Kernel6_5, kernel.Kernel6_6):
		return 168
	default:
		return 32
	}
}

func getFileFpathOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.IsUbuntuKernel() && kv.IsInRangeCloseOpen(kernel.Kernel6_5, kernel.Kernel6_6):
		return 152
	default:
		return 16
	}
}

func getMountIDOffset(kv *kernel.Version) uint64 {
	switch {
	case kv.IsSuseKernel() || kv.Code >= kernel.Kernel5_12:
		return 292
	case kv.Code != 0 && kv.Code < kernel.Kernel4_13:
		return 268
	default:
		return 284
	}
}

func getNetDeviceNameOffset(_ *kernel.Version) uint64 {
	return 0
}

func getRenameStructOldDentryOffset(_ *kernel.Version) uint64 {
	return 16
}

func getRenameStructNewDentryOffset(_ *kernel.Version) uint64 {
	return 40
}
