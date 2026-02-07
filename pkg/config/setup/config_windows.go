// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// defaultGuiPort is the default GUI port on Windows
	defaultGuiPort = 5002
)

// Exported default paths - sourced from defaultpaths package (the source of truth)
// These are used by external packages that need default paths for logging setup.
// For runtime path access, use defaultpaths getters
// Note: On Windows, defaultpaths.init() handles registry-based path customization.
var (
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
	// DefaultSystemProbeAddress is the default address to be used for connecting to the system probe
	DefaultSystemProbeAddress = `\\.\pipe\dd_system_probe`
	// DefaultDDAgentBin the process agent's binary
	DefaultDDAgentBin = "c:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"
	// InstallPath is the default install path for the agent
	InstallPath = "c:\\Program Files\\Datadog\\Datadog Agent"
)

func osinit() {
	// The config dir is configurable on Windows, so fetch the path from the registry
	// This updates the exported vars to reflect the actual ProgramData location
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultSecurityAgentLogFile = filepath.Join(pd, "logs", "security-agent.log")
		DefaultProcessAgentLogFile = filepath.Join(pd, "logs", "process-agent.log")
		DefaultUpdaterLogFile = filepath.Join(pd, "logs", "updater.log")
		DefaultOTelAgentLogFile = filepath.Join(pd, "logs", "otel-agent.log")
		DefaultHostProfilerLogFile = filepath.Join(pd, "logs", "host-profiler.log")
		DefaultPrivateActionRunnerLogFile = filepath.Join(pd, "logs", "private-action-runner.log")
		DefaultStreamlogsLogFile = filepath.Join(pd, "logs", "streamlogs_info", "streamlogs.log")
	}

	// The install path is configurable on Windows, so fetch the path from the registry
	// Do NOT use executable.Folder() or _here to calculate the path, some exe files are in different locations
	// so this can lead to an incorrect result.
	pd, err = winutil.GetProgramFilesDirForProduct("Datadog Agent")
	if err == nil {
		InstallPath = pd
		DefaultDDAgentBin = filepath.Join(InstallPath, "bin", "agent.exe")
	}

	// Fleet Automation
	pkgconfigmodel.AddOverrideFunc(FleetConfigOverride)
}

// FleetConfigOverride sets the fleet_policies_dir config value to the value set in the registry.
//
// This value tells the agent to load a config experiment from Fleet Automation.
//
// Linux sets this option with an environment variable in the experiment's systemd unit file,
// so we need a different approach for Windows. After the viper migration is complete, we can
// consider replacing this override with a Windows Registry config source.
func FleetConfigOverride(config pkgconfigmodel.Config) {
	// Prioritize the value set in the config file / env var
	if config.IsConfigured("fleet_policies_dir") {
		return
	}

	// value is not set, get the default value from the registry
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		"SOFTWARE\\Datadog\\Datadog Agent",
		registry.ALL_ACCESS)
	if err != nil {
		return
	}
	defer k.Close()
	val, _, err := k.GetStringValue("fleet_policies_dir")
	if err != nil {
		return
	}
	if val == "" {
		return
	}

	config.Set("fleet_policies_dir", val, pkgconfigmodel.SourceAgentRuntime)
}
