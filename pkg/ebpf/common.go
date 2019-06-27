package ebpf

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

var requiredKernelFuncs = []string{
	// Maps (3.18)
	"bpf_map_lookup_elem",
	"bpf_map_update_elem",
	"bpf_map_delete_elem",
	// kprobes (4.1)
	"bpf_probe_read",
	// Perf events (4.4)
	"bpf_perf_event_output",
	"bpf_perf_event_read",
}

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

	kallsyms, err := readKallsyms()
	if err != nil {
		log.Warnf("error reading /proc/kallsyms file: %s", err)
		// If we can't read the /proc/kallsyms file let's just return true to avoid blocking the tracer from running
		return true, nil
	}

	return verifyKernelFuncs(kallsyms), nil
}

func readKallsyms() (string, error) {
	procRoot := util.GetProcRoot()
	raw, err := ioutil.ReadFile(path.Join(procRoot, "kallsyms"))
	if err != nil {
		return "", errors.Wrapf(err, "error reading kallsyms file from proc dir: %s", procRoot)
	}

	return string(raw), nil
}

func verifyKernelFuncs(kallsyms string) bool {
	lines := strings.Split(kallsyms, "\n")
	funcs := make(map[string]struct{}, len(lines))

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		funcs[fields[2]] = struct{}{}
	}

	for _, f := range requiredKernelFuncs {
		if _, ok := funcs[f]; !ok {
			return false
		}
	}

	return true
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

func isCentOS(platform string) bool {
	return strings.Contains(platform, "centos")
}

func isUbuntu(platform string) bool {
	return strings.Contains(platform, "ubuntu")
}
