// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	defaultConfdPath            = "c:\\programdata\\datadog\\conf.d"
	defaultAdditionalChecksPath = "c:\\programdata\\datadog\\checks.d"
	defaultRunPath              = "c:\\programdata\\datadog\\run"
	defaultGuiPort              = 5002
	// DefaultUpdaterLogFile is the default updater log file
	DefaultUpdaterLogFile = "c:\\programdata\\datadog\\logs\\updater.log"
	// DefaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	DefaultSecurityAgentLogFile = "c:\\programdata\\datadog\\logs\\security-agent.log"
	// DefaultProcessAgentLogFile is the default process-agent log file
	DefaultProcessAgentLogFile = "C:\\ProgramData\\Datadog\\logs\\process-agent.log"
	// DefaultOTelAgentLogFile is the default otel-agent log file
	DefaultOTelAgentLogFile = "C:\\ProgramData\\Datadog\\logs\\otel-agent.log"
	// DefaultHostProfilerLogFile is the default host-profiler log file
	DefaultHostProfilerLogFile = "C:\\ProgramData\\Datadog\\logs\\host-profiler.log"
	// DefaultSystemProbeAddress is the default address to be used for connecting to the system probe
	DefaultSystemProbeAddress = `\\.\pipe\dd_system_probe`
	// defaultEventMonitorAddress is the default address to be used for connecting to the event monitor
	defaultEventMonitorAddress = "localhost:3337"
	// defaultSystemProbeLogFilePath is the default system probe log file
	defaultSystemProbeLogFilePath = "c:\\programdata\\datadog\\logs\\system-probe.log"
	// DefaultDDAgentBin the process agent's binary
	DefaultDDAgentBin = "c:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"
	// InstallPath is the default install path for the agent
	InstallPath = "c:\\Program Files\\Datadog\\Datadog Agent"
	// defaultStatsdSocket is the default Unix Domain Socket path on which statsd will listen
	defaultStatsdSocket = ""
	// defaultReceiverSocket is the default Unix Domain Socket path on which Trace agent will listen
	defaultReceiverSocket = ""
	//DefaultStreamlogsLogFile points to the stream logs log file that will be used if not configured
	DefaultStreamlogsLogFile = "c:\\programdata\\datadog\\logs\\streamlogs_info\\streamlogs.log"
)

func osinit() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultConfdPath = filepath.Join(pd, "conf.d")
		defaultAdditionalChecksPath = filepath.Join(pd, "checks.d")
		defaultRunPath = filepath.Join(pd, "run")
		DefaultSecurityAgentLogFile = filepath.Join(pd, "logs", "security-agent.log")
		defaultSystemProbeLogFilePath = filepath.Join(pd, "logs", "system-probe.log")
		DefaultProcessAgentLogFile = filepath.Join(pd, "logs", "process-agent.log")
		DefaultUpdaterLogFile = filepath.Join(pd, "logs", "updater.log")
		DefaultOTelAgentLogFile = filepath.Join(pd, "logs", "otel-agent.log")
		DefaultHostProfilerLogFile = filepath.Join(pd, "logs", "host-profiler.log")
	}

	// Agent binary
	if _here, err := executable.Folder(); err == nil {
		InstallPath = filepath.Join(_here, "..", "..")
		agentFilePath := filepath.Join(InstallPath, "embedded", "agent.exe")
		if _, err := os.Stat(agentFilePath); err == nil {
			DefaultDDAgentBin = agentFilePath
		}
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
