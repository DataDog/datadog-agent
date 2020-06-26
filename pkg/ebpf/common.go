package ebpf

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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
	// bpf_probe_read intentionally omitted since it was renamed in kernel 5.5
	// Perf events (4.4)
	"bpf_perf_event_output",
	"bpf_perf_event_read",
}

var (
	// ErrNotImplemented will be returned on non-linux environments like Windows and Mac OSX
	ErrNotImplemented = errors.New("BPF-based system probe not implemented on non-linux systems")
)

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
	return verifyOSVersion(currentKernelCode, platform, exclusionList)
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

// snakeToCapInitialCamel converts a snake case to Camel case with capital initial
func snakeToCapInitialCamel(s string) string {
	n := ""
	capNext := true
	for _, v := range s {
		if v >= 'A' && v <= 'Z' {
			n += string(v)
		}
		if v >= 'a' && v <= 'z' {
			if capNext {
				n += strings.ToUpper(string(v))
			} else {
				n += string(v)
			}
		}
		if v == '_' {
			capNext = true
		} else {
			capNext = false
		}
	}
	return n
}
