// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

// Package tags provides GPU-related host tags
package tags

import (
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// used for tests to mock the NVML library
var nvmlLibrary nvml.Interface
var mtx sync.Mutex

func ensureNvmlLibrary(getLibrary func(opts ...nvml.LibraryOption) nvml.Interface) error {
	// if the library is already initialized, return directly to avoid unnecessary locking
	if nvmlLibrary != nil {
		return nil
	}

	mtx.Lock()
	defer mtx.Unlock()

	// already initialized
	if nvmlLibrary != nil {
		return nil
	}

	lib := getLibrary()
	if ret := lib.Init(); ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("Failed to get gpu host tags, failed to initialize NVML: %v", nvml.ErrorString(ret))
	}

	nvmlLibrary = lib
	return nil
}

// getTags is the internal implementation of GetTags, exposed so that we can test with a mock NVML library
// to ensure we're properly handling library initialization errors
func getTags(getLibrary func(opts ...nvml.LibraryOption) nvml.Interface) []string {
	if err := ensureNvmlLibrary(getLibrary); err != nil {
		log.Warnf("Failed to get gpu host tags, failed to initialize NVML: %v", err)
		return nil
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

// GetTags returns gpu_host:true if any NVIDIA GPUs are present, nil otherwise
func GetTags() []string {
	return getTags(nvml.New)
}
