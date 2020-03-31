package ebpf

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
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

	// NativeEndian of the current host
	NativeEndian binary.ByteOrder
)

// KernelCodeToString translates a uint32 into a 'a.b.c' string
func KernelCodeToString(code uint32) string {
	// Kernel "a.b.c", the version number will be (a<<16 + b<<8 + c)
	a, b, c := code>>16, code>>8&0xff, code&0xff
	return fmt.Sprintf("%d.%d.%d", a, b, c)
}

// StringToKernelCode translates a 'a.b.c' string into a (a<<16 + b<<8 + c) number
func StringToKernelCode(str string) uint32 {
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
	if err == ErrNotImplemented {
		log.Infof("Could not detect OS, will assume supported.")
	} else if err != nil {
		return false, fmt.Sprintf("could not get kernel version: %s", err)
	}

	platform, err := util.GetPlatform()
	if err != nil {
		log.Warnf("error retrieving current platform: %s", err)
	} else {
		log.Infof("running on platform: %s", platform)
	}
	return VerifyOSVersion(currentKernelCode, platform, exclusionList)
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
		NativeEndian = binary.LittleEndian
	} else {
		NativeEndian = binary.BigEndian
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

// IsPre410Kernel compares current kernel version to the minimum kernel version(4.1.0) and see if it's older
func IsPre410Kernel(currentKernelCode uint32) bool {
	return currentKernelCode < StringToKernelCode("4.1.0")
}
