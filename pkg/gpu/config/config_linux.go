// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// MinimumKernelVersion indicates the minimum kernel version required for GPU monitoring
var MinimumKernelVersion kernel.Version

func init() {
	// we rely on ring buffer support for GPU monitoring, hence the minimal kernel version is 5.8.0
	MinimumKernelVersion = kernel.VersionCode(5, 8, 0)
}

// CheckGPUSupported checks if the host's kernel supports GPU monitoring
func CheckGPUSupported() error {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return fmt.Errorf("%w: could not determine the current kernel version: %w", ErrNotSupported, err)
	}

	if kversion < MinimumKernelVersion {
		return fmt.Errorf("%w: a Linux kernel version of %s or higher is required; we detected %s", ErrNotSupported, MinimumKernelVersion, kversion)
	}

	return nil
}
