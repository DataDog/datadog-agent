package ebpf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var (
	// Feature versions sourced from: https://github.com/iovisor/bcc/blob/master/docs/kernel-versions.md
	// Minimum kernel version -> max(3.15 - eBPF,
	//                               3.18 - tables/maps,
	//                               4.1 - kprobes,
	//                               4.3 - perf events)
	// 	                      -> 4.3
	minRequiredKernelCode = linuxKernelVersionCode(4, 3, 0)
)

var (
	// ErrNotImplemented will be returned on non-linux environments like Windows and Mac OSX
	ErrNotImplemented = errors.New("BPF-based system probe not implemented on non-linux systems")

	nativeEndian binary.ByteOrder
)

func kernelCodeToString(code uint32) string {
	// Kernel "a.b.c", the version number will be (a<<16 + b<<8 + c)
	a, b, c := code>>16, code>>8&0xf, code&0xf
	return fmt.Sprintf("%d.%d.%d", a, b, c)
}

func stringToKernelCode(str string) uint32 {
	var a, b, c uint32
	fmt.Sscanf(str, "%d.%d.%d", &a, &b, &c)
	return linuxKernelVersionCode(a, b, c)
}

// KERNEL_VERSION(a,b,c) = (a << 16) + (b << 8) + (c)
// Per https://github.com/torvalds/linux/blob/master/Makefile#L1187
func linuxKernelVersionCode(major, minor, patch uint32) uint32 {
	return (major << 16) + (minor << 8) + patch
}

// IsTracerSupportedByOS returns whether or not the current kernel version supports tracer functionality
func IsTracerSupportedByOS(exclusionList []string) (bool, error) {
	currentKernelCode, err := CurrentKernelVersion()
	if err != nil {
		return false, fmt.Errorf("could not get kernel version: %s", err)
	}

	platform, _ := util.GetPlatform()
	return verifyOSVersion(currentKernelCode, platform, exclusionList)
}

func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, error) {
	for _, version := range exclusionList {
		if code := stringToKernelCode(version); code == kernelCode {
			return false, fmt.Errorf(
				"current kernel version (%s) is in the exclusion list: %s (list: %+v)",
				kernelCodeToString(kernelCode),
				version,
				exclusionList,
			)
		}
	}

	if kernelCode < minRequiredKernelCode {
		return false, fmt.Errorf(
			"incompatible linux version. at least %s (%d) required, got %s (%d)",
			kernelCodeToString(minRequiredKernelCode),
			minRequiredKernelCode,
			kernelCodeToString(kernelCode),
			kernelCode,
		)
	}

	// Hardcoded exclusion list
	if platform == "" {
		// If we can't retrieve the platform just return true to avoid blocking the tracer from running
		return true, nil
	}

	if isUbuntu(platform) {
		if kernelCode >= linuxKernelVersionCode(4, 4, 119) && kernelCode <= linuxKernelVersionCode(4, 4, 126) {
			return false, fmt.Errorf("got ubuntu kernel %s with known bug on platform: %s, see: https://bugs.launchpad.net/ubuntu/+source/linux/+bug/1763454", kernelCodeToString(kernelCode), platform)
		}
	}

	return true, nil
}

// In lack of binary.NativeEndian ...
func init() {
	var i int32 = 0x01020304
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	b := *pb
	if b == 0x04 {
		nativeEndian = binary.LittleEndian
	} else {
		nativeEndian = binary.BigEndian
	}
}

func isUbuntu(platform string) bool {
	return strings.Contains(platform, "ubuntu")
}
