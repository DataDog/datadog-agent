// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

const (
	defaultConfdPath            = "/opt/datadog-agent/etc/conf.d"
	defaultAdditionalChecksPath = "/opt/datadog-agent/etc/checks.d"
	defaultRunPath              = "/opt/datadog-agent/run"
	defaultGuiPort              = 5002
	// DefaultUpdaterLogFile is the default updater log file
	DefaultUpdaterLogFile = "/opt/datadog-agent/logs/updater.log"
	// DefaultSecurityAgentLogFile points to the log file that will be used by the security-agent if not configured
	DefaultSecurityAgentLogFile = "/opt/datadog-agent/logs/security-agent.log"
	// DefaultProcessAgentLogFile is the default process-agent log file
	DefaultProcessAgentLogFile = "/opt/datadog-agent/logs/process-agent.log"
	// DefaultOTelAgentLogFile is the default otel-agent log file
	DefaultOTelAgentLogFile = "/opt/datadog-agent/logs/otel-agent.log"
	// DefaultHostProfilerLogFile is the default host-profiler log file
	DefaultHostProfilerLogFile = "/opt/datadog-agent/logs/host-profiler.log"
	// DefaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	DefaultSystemProbeAddress = "/opt/datadog-agent/run/sysprobe.sock"
	// defaultEventMonitorAddress is the default unix socket path to be used for connecting to the event monitor
	defaultEventMonitorAddress    = "/opt/datadog-agent/run/event-monitor.sock"
	defaultSystemProbeLogFilePath = "/opt/datadog-agent/logs/system-probe.log"
	// DefaultDDAgentBin the process agent's binary
	DefaultDDAgentBin = "/opt/datadog-agent/bin/agent/agent"
	// InstallPath is the default install path for the agent
	InstallPath = "/opt/datadog-agent"
	// defaultStatsdSocket is the default Unix Domain Socket path on which statsd will listen
	defaultStatsdSocket = ""
	// defaultReceiverSocket is the default Unix Domain Socket path on which Trace agent will listen
	defaultReceiverSocket = ""
	//DefaultStreamlogsLogFile points to the stream logs log file that will be used if not configured
	DefaultStreamlogsLogFile = "/opt/datadog-agent/logs/streamlogs_info/streamlogs.log"
)

// called by init in config.go, to ensure any os-specific config is done
// in time
func osinit() {
}

// FleetConfigOverride is a no-op on Darwin
func FleetConfigOverride(_ pkgconfigmodel.Config) {
}
