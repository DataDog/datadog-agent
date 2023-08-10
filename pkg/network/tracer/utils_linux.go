// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"fmt"

	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// IsTracerSupportedByOS returns whether or not the current kernel version supports tracer functionality
// along with some context on why it's not supported
func IsTracerSupportedByOS(exclusionList []string) (bool, error) {
	currentKernelCode, err := kernel.HostVersion()
	if err != nil {
		return false, fmt.Errorf("could not get kernel version: %s", err)
	}

	platform := hostMetadataUtils.GetPlatformName()
	log.Infof("running on platform: %s", platform)
	return verifyOSVersion(currentKernelCode, platform, exclusionList)
}

func verifyOSVersion(kernelCode kernel.Version, platform string, exclusionList []string) (bool, error) {
	for _, version := range exclusionList {
		if code := kernel.ParseVersion(version); code == kernelCode {
			return false, fmt.Errorf(
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
		return true, nil
	}

	// using eBPF causes kernel panic for linux kernel version 4.4.114 ~ 4.4.127
	if platform == "ubuntu" && kernelCode >= kernel.VersionCode(4, 4, 114) && kernelCode <= kernel.VersionCode(4, 4, 127) {
		return false, fmt.Errorf("Known bug for kernel %s on platform %s, see: \n- https://bugs.launchpad.net/ubuntu/+source/linux/+bug/1763454", kernelCode, platform)
	}
	return true, nil
}
