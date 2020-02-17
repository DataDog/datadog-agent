package ebpf

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"path"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

// Feature versions sourced from: https://github.com/iovisor/bcc/blob/master/docs/kernel-versions.md
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

// IsTracerSupportedByOS returns whether or not the current kernel version supports tracer functionality
// along with some context on why it's not supported
func IsTracerSupportedByOS(exclusionList []string) (bool, string) {
	currentKernelCode, err := CurrentKernelVersion()
	if err != nil {
		return false, fmt.Sprintf("could not get kernel version: %s", err)
	}

	platform, err := util.GetPlatform()
	if err != nil {
		log.Warnf("error retrieving current platform: %s", err)
	} else {
		log.Infof("running on platform: %s", platform)
	}
	return verifyOSVersion(currentKernelCode, platform, exclusionList)
}

func verifyOSVersion(kernelCode uint32, platform string, exclusionList []string) (bool, string) {
	for _, version := range exclusionList {
		if code := stringToKernelCode(version); code == kernelCode {
			return false, fmt.Sprintf(
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
		return true, ""
	}

	// using eBPF causes kernel panic for linux kernel version 4.4.114 ~ 4.4.127
	if isLinuxAWSUbuntu(platform) || isUbuntu(platform) {
		if kernelCode >= linuxKernelVersionCode(4, 4, 114) && kernelCode <= linuxKernelVersionCode(4, 4, 127) {
			return false, fmt.Sprintf("Known bug for kernel %s on platform %s, see: \n- https://bugs.launchpad.net/ubuntu/+source/linux/+bug/1763454", kernelCodeToString(kernelCode), platform)
		}
	}

	missing, err := verifyKernelFuncs(path.Join(util.GetProcRoot(), "kallsyms"))
	if err != nil {
		log.Warnf("error reading /proc/kallsyms file: %s (check your kernel version, current is: %s)", err, kernelCodeToString(kernelCode))
		// If we can't read the /proc/kallsyms file let's just return true to avoid blocking the tracer from running
		return true, ""
	}

	if len(missing) == 0 {
		return true, ""
	}

	errMsg := fmt.Sprintf("Kernel unsupported (%s) - ", kernelCodeToString(kernelCode))
	errMsg += fmt.Sprintf("required functions missing: %s", strings.Join(missing, ", "))
	return false, errMsg
}

func verifyKernelFuncs(path string) ([]string, error) {
	// Will hold the found functions
	found := make(map[string]bool, len(requiredKernelFuncs))
	for _, f := range requiredKernelFuncs {
		found[f] = false
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading kallsyms file from: %s", path)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		name := fields[2]
		if _, ok := found[name]; ok {
			found[name] = true
		}
	}

	missing := []string{}
	for probe, b := range found {
		if !b {
			missing = append(missing, probe)
		}
	}

	return missing, nil
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
	return strings.Contains(strings.ToLower(platform), "ubuntu")
}

func isLinuxAWSUbuntu(platform string) bool {
	return strings.Contains(strings.ToLower(platform), "aws") && isUbuntu(platform)
}

func isCentOS(platform string) bool {
	return strings.Contains(strings.ToLower(platform), "centos")
}

func isRHEL(platform string) bool {
	p := strings.ToLower(platform)
	return strings.Contains(p, "redhat") || strings.Contains(p, "red hat") || strings.Contains(p, "rhel")
}

func isRHELOrCentOS() (bool, error) {
	platform, err := util.GetPlatform()
	if err != nil {
		return false, err
	}
	return isCentOS(platform) || isRHEL(platform), nil
}

// isPre410Kernel compares current kernel version to the minimum kernel version(4.1.0) and see if it's older
func isPre410Kernel(currentKernelCode uint32) bool {
	return currentKernelCode < stringToKernelCode("4.1.0")
}
