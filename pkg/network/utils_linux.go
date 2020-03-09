// +build linux_bpf

package network

import (
	"fmt"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
