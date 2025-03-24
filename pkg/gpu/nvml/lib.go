// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

// Package nvml provides some convenience functions for using the NVML library.
package nvml

import (
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
)

type nvmlCache struct {
	lib           nvml.Interface
	isInitialized bool
	mu            sync.Mutex
}

type nvmlCacheInitOpts struct {
	nvmlNewFunc func(opts ...nvml.LibraryOption) nvml.Interface
}

// ensureInitWithOpts initializes the NVML library with the given options (used for testing)
func (c *nvmlCache) ensureInitWithOpts(opts nvmlCacheInitOpts) error {
	// If the library is already initialized, return nil without locking
	if c.isInitialized {
		return nil
	}

	// Lock the mutex to ensure thread-safe initialization
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check again after locking to ensure no race condition
	if c.isInitialized {
		return nil
	}

	if c.lib == nil {
		config := gpuconfig.New()
		c.lib = opts.nvmlNewFunc(nvml.WithLibraryPath(config.NVMLLibraryPath))
	}

	ret := c.lib.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("error initializing NVML library: %s", nvml.ErrorString(ret))
	}

	c.isInitialized = true

	return nil
}

// ensureInit initializes the NVML library with the default options.
func (c *nvmlCache) ensureInit() error {
	return c.ensureInitWithOpts(nvmlCacheInitOpts{nvmlNewFunc: nvml.New})
}

var singleton nvmlCache

// GetNvmlLib returns the NVML library instance.
// It will initialize the library if it is not already initialized.
func GetNvmlLib() (nvml.Interface, error) {
	if err := singleton.ensureInit(); err != nil {
		return nil, err
	}

	return singleton.lib, nil
}
