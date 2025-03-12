// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package nvml

import (
	"fmt"
	"sync"

	"github.com/NVIDIA/go-nvml/nvml"

	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
)

type nvmlCache struct {
	lib           nvml.Interface
	isInitialized bool
	mu            sync.Mutex
}

func (c *nvmlCache) ensureInit() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isInitialized {
		return nil
	}

	if c.lib == nil {
		config := gpuconfig.New()
		c.lib = nvml.New(nvml.WithLibraryPath(config.NVMLLibraryPath))
	}

	ret := c.lib.Init()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("error initializing NVML library: %s", nvml.ErrorString(ret))
	}

	c.isInitialized = true

	return nil
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
