// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"fmt"
	"path"
	"strings"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// IsTracerSupportedByOS returns whether or not the current kernel version supports tracer functionality
// along with some context on why it's not supported
func IsTracerSupportedByOS(exclusionList []string) (bool, string) {
	currentKernelCode, err := kernel.HostVersion()
	if err != nil {
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

	missing, err := ddebpf.VerifyKernelFuncs(path.Join(util.GetProcRoot(), "kallsyms"), requiredKernelFuncs)
	if err != nil {
		log.Warnf("error reading /proc/kallsyms file: %s (check your kernel version, current is: %s)", err, kernelCode)
		// If we can't read the /proc/kallsyms file let's just return true to avoid blocking the tracer from running
		return true, ""
	}
	if len(missing) == 0 {
		return true, ""
	}
	errMsg := fmt.Sprintf("Kernel unsupported (%s) - ", kernelCode)

	var missingFuncs []string
	for f := range missing {
		missingFuncs = append(missingFuncs, f)
	}
	errMsg += fmt.Sprintf("required functions missing: %s", strings.Join(missingFuncs, ", "))
	return false, errMsg
}
