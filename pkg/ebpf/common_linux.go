// +build linux_bpf

package ebpf

import (
	"encoding/binary"
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
