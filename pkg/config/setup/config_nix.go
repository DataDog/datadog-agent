// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly || aix

package setup

import (
	"path/filepath"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

const (
	// defaultGuiPort is the default GUI port (-1 means disabled on Linux)
	defaultGuiPort = -1
)

// Exported default paths - sourced from defaultpaths package (the source of truth)
// These are used by external packages that need default paths for logging setup.
// For runtime path access, use defaultpaths getters
var (
	InstallPath = defaultpaths.DefaultInstallPath
	// DefaultUpdaterLogFile is the default updater log file
	DefaultUpdaterLogFile = defaultpaths.DefaultUpdaterLogFile
	// DefaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	DefaultSecurityAgentLogFile = defaultpaths.DefaultSecurityAgentLogFile
	// DefaultProcessAgentLogFile is the default process-agent log file
	DefaultProcessAgentLogFile = defaultpaths.DefaultProcessAgentLogFile
	// DefaultOTelAgentLogFile is the default otel-agent log file
	DefaultOTelAgentLogFile = defaultpaths.DefaultOTelAgentLogFile
	// DefaultHostProfilerLogFile is the default host-profiler log file
	DefaultHostProfilerLogFile = defaultpaths.DefaultHostProfilerLogFile
	// DefaultPrivateActionRunnerLogFile is the default private-action-runner log file
	DefaultPrivateActionRunnerLogFile = defaultpaths.DefaultPrivateActionRunnerLogFile
	// DefaultStreamlogsLogFile points to the stream logs log file that will be used if not configured
	DefaultStreamlogsLogFile = defaultpaths.DefaultStreamlogsLogFile
	// DefaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	DefaultSystemProbeAddress = defaultpaths.DefaultSystemProbeAddress
	// DefaultDDAgentBin the process agent's binary
	DefaultDDAgentBin = filepath.Join(defaultpaths.DefaultBinPath, "agent")
)

// called by init in config.go, to ensure any os-specific config is done
// in time
func osinit() {
	InstallPath = defaultpaths.GetInstallPath()

	DefaultDDAgentBin = filepath.Join(InstallPath, "bin", "agent")
	DefaultSystemProbeAddress = filepath.Join(InstallPath, "sysprobe.sock")
	defaultSystemProbeBPFDir = filepath.Join(InstallPath, "embedded/share/system-probe/ebpf")
}

// FleetConfigOverride is a no-op on Linux
func FleetConfigOverride(_ pkgconfigmodel.Config) {
}
