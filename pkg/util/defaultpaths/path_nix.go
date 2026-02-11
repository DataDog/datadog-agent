// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build netbsd || openbsd || solaris || dragonfly || linux

package defaultpaths

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

// Exported default path constants for use in BindEnvAndSetDefault and similar config registration.
// These are the raw, untransformed paths. Use getter functions for runtime transformed paths.
const (
	// DefaultInstallPath is the default install path for the agent
	// It might be overridden at build time
	DefaultInstallPath = "/opt/datadog-agent"
	// DefaultRunPath is the default runtime directory for the agent
	DefaultRunPath = "/opt/datadog-agent/run"
	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "/etc/datadog-agent"
	// DefaultConfdPath points to the folder containing integration configuration files
	DefaultConfdPath = "/etc/datadog-agent/conf.d"
	// DefaultAdditionalChecksPath points to the folder containing custom python integration files
	DefaultAdditionalChecksPath = "/etc/datadog-agent/checks.d"
	// DefaultPyChecksPath points to the folder containing preinstalled integrations with the agent
	DefaultPyChecksPath = "/opt/datadog-agent/checks.d"
	// DefaultBinPath is the installation folder for agent binaries
	DefaultBinPath = "/opt/datadog-agent/bin/agent"
	// CheckFlareDirectory a flare friendly location for checks to be written
	DefaultCheckFlareDirectory = "/var/log/datadog/checks/"
	// JMXFlareDirectory a flare friendly location for jmx command logs to be written
	DefaultJMXFlareDirectory = "/var/log/datadog/jmxinfo/"

	// Log files

	// LogFile points to the log file that will be used if not configured
	DefaultLogFile = "/var/log/datadog/agent.log"
	// DCALogFile points to the log file that will be used if not configured
	DefaultDCALogFile = "/var/log/datadog/cluster-agent.log"
	// JmxLogFile points to the jmx fetch log file that will be used if not configured
	DefaultJmxLogFile = "/var/log/datadog/jmxfetch.log"
	// DogstatsDLogFile points to the dogstatsd stats log file that will be used if not configured
	DefaultDogstatsDLogFile         = "/var/log/datadog/dogstatsd_info/dogstatsd-stats.log"
	DefaultDogstatsDProtocolLogFile = "/var/log/datadog/dogstatsd_info/dogstatsd-stats.log"
	// StreamlogsLogFile points to the stream logs log file that will be used if not configured
	DefaultStreamlogsLogFile = "/var/log/datadog/streamlogs_info/streamlogs.log"
	// DefaultUpdaterLogFile is the default log file location for updater
	DefaultUpdaterLogFile = "/var/log/datadog/updater.log"
	// DefaultTraceAgentLogFile is the default log file location for trace agent
	DefaultTraceAgentLogFile = "/var/log/datadog/trace-agent.log"
	// DefaultSecurityAgentLogFile is the default log file location for security agent
	DefaultSecurityAgentLogFile = "/var/log/datadog/security-agent.log"
	// DefaultProcessAgentLogFile is the default log file location for process agent
	DefaultProcessAgentLogFile = "/var/log/datadog/process-agent.log"
	// DefaultSystemProbeLogFile is the default log file location for the system probe
	DefaultSystemProbeLogFile = "/var/log/datadog/system-probe.log"
	// DefaultOTelAgentLogFile is the default log file location for the otel agent
	DefaultOTelAgentLogFile = "/var/log/datadog/otel-agent.log"
	// DefaultHostProfilerLogFile is the default log file location for the host profiler
	DefaultHostProfilerLogFile = "/var/log/datadog/host-profiler.log"
	// DefaultPrivateActionRunnerLogFile is the default log file location for the private action runner
	DefaultPrivateActionRunnerLogFile = "/var/log/datadog/private-action-runner.log"

	// Sockets

	// DefaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	DefaultSystemProbeAddress = "/opt/datadog-agent/run/sysprobe.sock"
	// DefaultStatsdSocket is the default dogstatsd socket path, it is at /var/run/datadog for historical reasons
	DefaultStatsdSocket = "/var/run/datadog/dsd.socket"
	// DefaultStatsdSocket is the default trace agent socket path, it is at /var/run/datadog for historical reasons
	DefaultReceiverSocket = "/var/run/datadog/apm.socket"
)

var (
	// utility variables
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
	return getInstallPathFromExecutable(_here)
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

// getInstallPathFromExecutable will go up the directory chain from start in search of a .install_root file.
// That directory will become the install path.
//
// If not found, returns the default InstallPath.
func getInstallPathFromExecutable(start string) string {
	// Start from the current directory
	currentDir := start

	for {
		installRoot := filepath.Join(currentDir, ".install_root")
		if _, err := os.Stat(installRoot); err == nil {
			return currentDir
		}
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}
		currentDir = parentDir
	}
	return DefaultInstallPath // Fallback to the default install path
}
