// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

// Package nvml provides some convenience functions for using the NVML library.
package nvml

import (
	"fmt"
	"strings"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

type nvmlCache struct {
	lib           nvml.Interface
	isInitialized bool
	mu            sync.Mutex
}

// ensureInitWithOpts initializes the NVML library with the given options (used for testing)
func (c *nvmlCache) ensureInitWithOpts(nvmlNewFunc func(opts ...nvml.LibraryOption) nvml.Interface) (err error) {
	// If the library is already initialized, return nil without locking
	if c.isInitialized {
		return nil
	}

	// Lock the mutex to ensure thread-safe initialization
	c.mu.Lock()
	defer func() {
		// Set the initialized state before unlocking but in the defer(), so that we never
		// set it before the library is completely initialized.
		// err is the error returned by the function as declared above
		c.isInitialized = err == nil
		c.mu.Unlock()
	}()

	// Check again after locking to ensure no race condition
	if c.isInitialized {
		return nil
	}

	if c.lib == nil {
		var libpath string
		if flavor.GetFlavor() == flavor.SystemProbe {
			cfg := pkgconfigsetup.SystemProbe()
			// Use the config directly here to avoid importing the entire gpu
			// config package, which includes system-probe specific imports
			libpath = cfg.GetString(strings.Join([]string{consts.GPUNS, "nvml_lib_path"}, "."))
		} else {
			cfg := pkgconfigsetup.Datadog()
			libpath = cfg.GetString("nvml_lib_path")
		}

		c.lib = nvmlNewFunc(nvml.WithLibraryPath(libpath))
		if c.lib == nil {
			return fmt.Errorf("failed to create NVML library")
		}
	}

	ret := c.lib.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("error initializing NVML library: %s", nvml.ErrorString(ret))
	}

	return nil
}

// ensureInit initializes the NVML library with the default options.
func (c *nvmlCache) ensureInit() error {
	return c.ensureInitWithOpts(nvml.New)
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
