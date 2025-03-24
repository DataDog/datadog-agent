// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

// Package tags provides GPU-related host tags
package tags

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// used for tests to mock the NVML library
var nvmlLibrary nvml.Interface

// GetTags returns gpu_host:true if any NVIDIA GPUs are present, nil otherwise
func GetTags() []string {
	if nvmlLibrary == nil {
		nvmlLibrary = nvml.New()
		if ret := nvmlLibrary.Init(); ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
			log.Warnf("Failed to get gpu host tags, failed to initialize NVML: %v", nvml.ErrorString(ret))
			return nil
		}
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
