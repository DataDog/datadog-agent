// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package config provides the GPU monitoring config.
package config

import (
	"errors"
	"time"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// GPUNS is the namespace for the GPU monitoring probe.
const GPUNS = "gpu_monitoring"

// ErrNotSupported is the error returned if GPU monitoring is not supported on this platform
var ErrNotSupported = errors.New("GPU Monitoring is not supported")

// Config holds the configuration for the GPU monitoring probe.
type Config struct {
	ebpf.Config
	// Enabled indicates whether the GPU monitoring probe is enabled.
	Enabled bool
	// ScanTerminatedProcessesInterval is the interval at which the probe scans for terminated processes.
	ScanTerminatedProcessesInterval time.Duration
	// InitialProcessSync indicates whether the probe should sync the process list on startup.
	InitialProcessSync bool
	// NVMLLibraryPath is the path of the native libnvidia-ml.so library
	NVMLLibraryPath string
}

// New generates a new configuration for the GPU monitoring probe.
func New() *Config {
	spCfg := pkgconfigsetup.SystemProbe()
	return &Config{
		Config:                          *ebpf.NewConfig(),
		ScanTerminatedProcessesInterval: time.Duration(spCfg.GetInt(sysconfig.FullKeyPath(GPUNS, "process_scan_interval_seconds"))) * time.Second,
		InitialProcessSync:              spCfg.GetBool(sysconfig.FullKeyPath(GPUNS, "initial_process_sync")),
		NVMLLibraryPath:                 spCfg.GetString(sysconfig.FullKeyPath(GPUNS, "nvml_lib_path")),
		Enabled:                         spCfg.GetBool(sysconfig.FullKeyPath(GPUNS, "enabled")),
	}
}
