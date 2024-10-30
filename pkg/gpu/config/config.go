// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package config provides the GPU monitoring config.
package config

import (
	"errors"
	"fmt"
	"time"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// GPUNS is the namespace for the GPU monitoring probe.
const GPUNS = "gpu_monitoring"

// ErrNotSupported is the error returned if GPU monitoring is not supported on this platform
var ErrNotSupported = errors.New("GPU Monitoring is not supported")

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

// Config holds the configuration for the GPU monitoring probe.
type Config struct {
	ebpf.Config
	// ScanTerminatedProcessesInterval is the interval at which the probe scans for terminated processes.
	ScanTerminatedProcessesInterval time.Duration
	// InitialProcessSync indicates whether the probe should sync the process list on startup.
	InitialProcessSync bool
	// NVMLLibraryPath is the path of the native libnvidia-ml.so library
	NVMLLibraryPath string
}

// NewConfig generates a new configuration for the GPU monitoring probe.
func NewConfig() *Config {
	spCfg := pkgconfigsetup.SystemProbe()
	return &Config{
		Config:                          *ebpf.NewConfig(),
		ScanTerminatedProcessesInterval: time.Duration(spCfg.GetInt(sysconfig.FullKeyPath(GPUNS, "process_scan_interval_seconds"))) * time.Second,
		InitialProcessSync:              spCfg.GetBool(sysconfig.FullKeyPath(GPUNS, "initial_process_sync")),
		NVMLLibraryPath:                 spCfg.GetString(sysconfig.FullKeyPath(GPUNS, "nvml_lib_path")),
	}
}
