// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags provides GPU-related host tags
package tags

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

var nvmlLibrary nvml.Interface

// GetTags returns gpu_host:true if any NVIDIA GPUs are present, nil otherwise
func GetTags() []string {
	if nvmlLibrary == nil {
		nvmlLibrary = nvml.New()
		if ret := nvmlLibrary.Init(); ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
			log.Warnf("Failed to get gpu host tags, failed to initialize NVML: %v", nvml.ErrorString(ret))
			return nil
		}
		// We don't call nvml.Shutdown() because the host tags will be queried multiple times
		// during the agent's lifetime. We'll let the process cleanup handle the shutdown.
	}

	count, ret := nvmlLibrary.DeviceGetCount()
	if ret != nvml.SUCCESS {
		log.Warnf("Failed to get gpu host tags, couldn't assess number of devices: %v", nvml.ErrorString(ret))
		return nil
	}

	if count > 0 {
		return []string{"gpu_host:true"}
	}

	return nil
}
