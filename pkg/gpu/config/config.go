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
	"github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
)

// ErrNotSupported is the error returned if GPU monitoring is not supported on this platform
var ErrNotSupported = errors.New("GPU Monitoring is not supported")

// Config holds the configuration for the GPU monitoring probe.
type Config struct {
	ebpf.Config
	// Enabled indicates whether the GPU monitoring probe is enabled.
	Enabled bool
	// ScanProcessesInterval is the interval at which the probe scans for new or terminated processes.
	ScanProcessesInterval time.Duration
	// InitialProcessSync indicates whether the probe should sync the process list on startup.
	InitialProcessSync bool
	// ConfigureCgroupPerms indicates whether the probe should configure cgroup permissions for GPU monitoring
	ConfigureCgroupPerms bool
	// EnableFatbinParsing indicates whether the probe should enable fatbin parsing.
	EnableFatbinParsing bool
}

// New generates a new configuration for the GPU monitoring probe.
func New() *Config {
	spCfg := pkgconfigsetup.SystemProbe()
	return &Config{
		Config:                *ebpf.NewConfig(),
		ScanProcessesInterval: time.Duration(spCfg.GetInt(sysconfig.FullKeyPath(consts.GPUNS, "process_scan_interval_seconds"))) * time.Second,
		InitialProcessSync:    spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "initial_process_sync")),
		Enabled:               spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "enabled")),
		ConfigureCgroupPerms:  spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "configure_cgroup_perms")),
		EnableFatbinParsing:   spCfg.GetBool(sysconfig.FullKeyPath(consts.GPUNS, "enable_fatbin_parsing")),
	}
}
