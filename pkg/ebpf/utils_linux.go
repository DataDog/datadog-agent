// +build linux_bpf

package ebpf

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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

	var missing []string
	for probe, b := range found {
		if !b {
			missing = append(missing, probe)
		}
	}

	return missing, nil
}

// IsTracerSupportedByOS returns whether or not the current kernel version supports tracer functionality
// along with some context on why it's not supported
func IsTracerSupportedByOS(exclusionList []string) (bool, string) {
	currentKernelCode, err := kernel.HostVersion()
	if err == ErrNotImplemented {
		log.Infof("Could not detect OS, will assume supported.")
	} else if err != nil {
		return false, fmt.Sprintf("could not get kernel version: %s", err)
	}

	hostInfo := host.GetStatusInformation()
	log.Infof("running on platform: %s", hostInfo.Platform)
	return verifyOSVersion(currentKernelCode, hostInfo.Platform, exclusionList)
}

func verifyOSVersion(kernelCode kernel.Version, platform string, exclusionList []string) (bool, string) {
	for _, version := range exclusionList {
		if code := kernel.ParseVersion(version); code == kernelCode {
			return false, fmt.Sprintf(
				"current kernel version (%s) is in the exclusion list: %s (list: %+v)",
				kernelCode,
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
	if platform == "ubuntu" && kernelCode >= kernel.VersionCode(4, 4, 114) && kernelCode <= kernel.VersionCode(4, 4, 127) {
		return false, fmt.Sprintf("Known bug for kernel %s on platform %s, see: \n- https://bugs.launchpad.net/ubuntu/+source/linux/+bug/1763454", kernelCode, platform)
	}

	missing, err := verifyKernelFuncs(path.Join(util.GetProcRoot(), "kallsyms"))
	if err != nil {
		log.Warnf("error reading /proc/kallsyms file: %s (check your kernel version, current is: %s)", err, kernelCode)
		// If we can't read the /proc/kallsyms file let's just return true to avoid blocking the tracer from running
		return true, ""
	}
	if len(missing) == 0 {
		return true, ""
	}
	errMsg := fmt.Sprintf("Kernel unsupported (%s) - ", kernelCode)
	errMsg += fmt.Sprintf("required functions missing: %s", strings.Join(missing, ", "))
	return false, errMsg
}
