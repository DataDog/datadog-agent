// +build linux_bpf

package ebpf

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unsafe"
)

var (
	nativeEndian binary.ByteOrder
)

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

func isLinuxAWSUbuntu(platform string) bool {
	return strings.Contains(strings.ToLower(platform), "aws") && isUbuntu(platform)
}

func isUbuntu(platform string) bool {
	return strings.Contains(strings.ToLower(platform), "ubuntu")
}

func isCentOS(platform string) bool {
	return strings.Contains(strings.ToLower(platform), "centos")
}

func isRHEL(platform string) bool {
	p := strings.ToLower(platform)
	return strings.Contains(p, "redhat") || strings.Contains(p, "red hat") || strings.Contains(p, "rhel")
}

// isPre410Kernel compares current kernel version to the minimum kernel version(4.1.0) and see if it's older
func isPre410Kernel(currentKernelCode uint32) bool {
	return currentKernelCode < stringToKernelCode("4.1.0")
}

func kernelCodeToString(code uint32) string {
	// Kernel "a.b.c", the version number will be (a<<16 + b<<8 + c)
	a, b, c := code>>16, code>>8&0xff, code&0xff
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
