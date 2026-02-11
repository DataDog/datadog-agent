// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultpaths

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

const (
	// DefaultInstallPath is the default install path for the agent
	// It might be overridden at build time
	DefaultInstallPath = "/opt/datadog-agent"
	// DefaultRunPath is the default runtime directory for the agent
	DefaultRunPath = "/opt/datadog-agent/run"
	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "/opt/datadog-agent/etc"
	// DefaultConfdPath points to the folder containing integration configuration files
	DefaultConfdPath = "/opt/datadog-agent/etc/conf.d"
	// DefaultAdditionalChecksPath points to the folder containing custom python integration files
	DefaultAdditionalChecksPath = "/opt/datadog-agent/etc/checks.d"
	// DefaultPyChecksPath points to the folder containing preinstalled integrations with the agent
	DefaultPyChecksPath = "/opt/datadog-agent/checks.d"
	// DefaultBinPath is the installation folder for agent binaries
	DefaultBinPath = "/opt/datadog-agent/bin/agent"
	// CheckFlareDirectory a flare friendly location for checks to be written
	DefaultCheckFlareDirectory = "/opt/datadog-agent/logs/checks/"
	// JMXFlareDirectory a flare friendly location for jmx command logs to be written
	DefaultJMXFlareDirectory = "/opt/datadog-agent/logs/jmxinfo/"

	// Log files

	// LogFile points to the log file that will be used if not configured
	DefaultLogFile = "/opt/datadog-agent/logs/agent.log"
	// DCALogFile points to the log file that will be used if not configured
	DefaultDCALogFile = "/opt/datadog-agent/logs/cluster-agent.log"
	// JmxLogFile points to the jmx fetch log file that will be used if not configured
	DefaultJmxLogFile = "/opt/datadog-agent/logs/jmxfetch.log"
	// DogstatsDLogFile points to the dogstatsd stats log file that will be used if not configured
	DefaultDogstatsDLogFile         = "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log"
	DefaultDogstatsDProtocolLogFile = "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log"
	// StreamlogsLogFile points to the stream logs log file that will be used if not configured
	DefaultStreamlogsLogFile = "/opt/datadog-agent/logs/streamlogs_info/streamlogs.log"
	// DefaultUpdaterLogFile is the default log file location for updater
	DefaultUpdaterLogFile = "/opt/datadog-agent/logs/updater.log"
	// DefaultTraceAgentLogFile is the default log file location for trace agent
	DefaultTraceAgentLogFile = "/opt/datadog-agent/logs/trace-agent.log"
	// DefaultSecurityAgentLogFile is the default log file location for security agent
	DefaultSecurityAgentLogFile = "/opt/datadog-agent/logs/security-agent.log"
	// DefaultProcessAgentLogFile is the default log file location for process agent
	DefaultProcessAgentLogFile = "/opt/datadog-agent/logs/process-agent.log"
	// DefaultSystemProbeLogFile is the default log file location for the system probe
	DefaultSystemProbeLogFile = "/opt/datadog-agent/logs/system-probe.log"
	// DefaultOTelAgentLogFile is the default log file location for the otel agent
	DefaultOTelAgentLogFile = "/opt/datadog-agent/logs/otel-agent.log"
	// DefaultHostProfilerLogFile is the default log file location for the host profiler
	DefaultHostProfilerLogFile = "/opt/datadog-agent/logs/host-profiler.log"
	// DefaultPrivateActionRunnerLogFile is the default log file location for the private action runner
	DefaultPrivateActionRunnerLogFile = "/opt/datadog-agent/logs/private-action-runner.log"

	// Sockets

	// DefaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	DefaultSystemProbeAddress = "/opt/datadog-agent/run/sysprobe.sock"
	// DefaultStatsdSocket is the default dogstatsd socket path, it is empty on darwin
	DefaultStatsdSocket = ""
	// DefaultStatsdSocket is the default trace agent socket path, it is empty on darwin
	DefaultReceiverSocket = ""

	// DefaultDDAgentBin the process agent's binary
	DefaultDDAgentBin = "/opt/datadog-agent/bin/agent/agent"
)

var (
	_here, _ = executable.Folder()

	// PyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	PyChecksPath = filepath.Join(_here, "..", "..", "checks.d")
	// DistPath holds the path to the folder containing distribution files
	distPath = filepath.Join(_here, "dist")
)

// GetDistPath returns the fully qualified path to the 'dist' directory
func GetDistPath() string {
	return distPath
}

// GetInstallPath returns the fully qualified path to the datadog-agent executable
func GetInstallPath() string {
	return _here
}

// GetDefaultRunPath returns the path to the run directory
func GetDefaultRunPath() string {
	return DefaultRunPath
}

// GetDefaultConfPath returns the path to the folder containing datadog.yaml
func GetDefaultConfPath() string {
	return DefaultConfPath
}

// GetDefaultConfdPath returns the path to the conf.d directory
func GetDefaultConfdPath() string {
	return DefaultConfdPath
}

// GetDefaultAdditionalChecksPath returns the path to the checks.d directory
func GetDefaultAdditionalChecksPath() string {
	return DefaultAdditionalChecksPath
}

// GetDefaultPyChecksPath returns the path to the python checks directory
func GetDefaultPyChecksPath() string {
	return DefaultPyChecksPath
}

// GetDefaultPidFilePath returns the path to the agent PID file
func GetDefaultPidFilePath() string {
	return filepath.Join(GetDefaultRunPath(), "datadog-agent.pid")
}

// GetBinPath returns the directory containing the agent executable.
// This is used by code that needs to find files relative to the executable location.
func GetBinPath() string {
	return DefaultBinPath
}

// GetDefaultRunPath returns the path to the run directory
func GetDefaultStatsdSocket() string {
	return DefaultStatsdSocket
}

// GetDefaultReceiverSocket returns the path to the APM receiver Unix socket
func GetDefaultReceiverSocket() string {
	return DefaultReceiverSocket
}

// GetDefaultJMXFlareDirectory returns the path to the JMX flare directory
func GetDefaultJMXFlareDirectory() string {
	return DefaultJMXFlareDirectory
}

// GetDefaultCheckFlareDirectory returns the path to the check flare directory
func GetDefaultCheckFlareDirectory() string {
	return DefaultCheckFlareDirectory
}

// GetDefaultSystemProbeLogFile returns the path to the system-probe log file
func GetDefaultSystemProbeLogFile() string {
	return DefaultSystemProbeLogFile
}

// GetDefaultPrivateActionRunnerLogFile returns the path to the private-action-runner log file
func GetDefaultPrivateActionRunnerLogFile() string {
	return DefaultPrivateActionRunnerLogFile
}

// GetDefaultHostProfilerLogFile returns the path to the host-profiler log file
func GetDefaultHostProfilerLogFile() string {
	return DefaultHostProfilerLogFile
}

// GetDefaultOTelAgentLogFile returns the path to the otel-agent log file
func GetDefaultOTelAgentLogFile() string {
	return DefaultOTelAgentLogFile
}

// GetDefaultProcessAgentLogFile returns the path to the process-agent log file
func GetDefaultProcessAgentLogFile() string {
	return DefaultProcessAgentLogFile
}

// GetDefaultSecurityAgentLogFile returns the path to the security-agent log file
func GetDefaultSecurityAgentLogFile() string {
	return DefaultSecurityAgentLogFile
}

// GetDefaultUpdaterLogFile returns the path to the updater log file
func GetDefaultUpdaterLogFile() string {
	return DefaultUpdaterLogFile
}

// GetDefaultStreamlogsLogFile returns the path to the streamlogs log file
func GetDefaultStreamlogsLogFile() string {
	return DefaultStreamlogsLogFile
}

// GetDefaultTraceAgentLogFile returns the path to the trace-agent log file
func GetDefaultTraceAgentLogFile() string {
	return DefaultTraceAgentLogFile
}

// GetDefaultDogstatsDServiceLogFile returns the path to the dogstatsd service log file
func GetDefaultDogstatsDServiceLogFile() string {
	return DefaultDogstatsDLogFile
}

// GetDefaultDogstatsDProtocolLogFile returns the path to the DogStatsD protocol stats log file
func GetDefaultDogstatsDProtocolLogFile() string {
	return DefaultDogstatsDProtocolLogFile
}

// GetDefaultJmxLogFile returns the path to the jmxfetch log file
func GetDefaultJmxLogFile() string {
	return DefaultJmxLogFile
}

// GetDefaultDCALogFile returns the path to the cluster-agent log file
func GetDefaultDCALogFile() string {
	return DefaultDCALogFile
}

// GetDefaultLogFile returns the path to the agent log file
func GetDefaultLogFile() string {
	return DefaultLogFile
}
