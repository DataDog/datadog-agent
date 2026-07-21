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
	// defaultCommonRoot is the default path used when DD_COMMON_ROOT is set but empty
	defaultCommonRoot = "/opt/datadog-agent"
	// defaultRunPath is the default runtime directory for the agent
	defaultRunPath = "/opt/datadog-agent/run"
	// defaultConfPath points to the folder containing datadog.yaml
	defaultConfPath = "/opt/datadog-agent/etc"
	// defaultLogPath points to the log folder that will be used if not configured
	defaultLogPath = "/opt/datadog-agent/logs"
	// defaultPyChecksPath points to the folder containing preinstalled integrations with the agent
	defaultPyChecksPath = "/opt/datadog-agent/checks.d"
	// CheckFlareDirectory a flare friendly location for checks to be written
	defaultCheckFlareDirectory = defaultLogPath + "/checks/"
	// JMXFlareDirectory a flare friendly location for jmx command logs to be written
	defaultJMXFlareDirectory = defaultLogPath + "/jmxinfo/"

	// Log files

	// LogFile points to the log file that will be used if not configured
	defaultLogFile = defaultLogPath + "/agent.log"
	// DCALogFile points to the log file that will be used if not configured
	defaultDCALogFile = defaultLogPath + "/cluster-agent.log"
	// JmxLogFile points to the jmx fetch log file that will be used if not configured
	defaultJmxLogFile = defaultLogPath + "/jmxfetch.log"
	// defaultDogstatsDServiceLogFile points to the old datadog.conf dogstatsd_log_file location for running dogstatsd in as a standalone service
	defaultDogstatsDServiceLogFile = "/var/log/datadog/dogstatsd.log"
	// defaultDogstatsDProtocolLogFile points to the dogstatsd stats log file that will be used if not configured
	defaultDogstatsDProtocolLogFile = defaultLogPath + "/dogstatsd_info/dogstatsd-stats.log"
	// StreamlogsLogFile points to the stream logs log file that will be used if not configured
	defaultStreamlogsLogFile = defaultLogPath + "/streamlogs_info/streamlogs.log"
	// defaultUpdaterLogFile is the default log file location for updater
	defaultUpdaterLogFile = defaultLogPath + "/updater.log"
	// defaultTraceAgentLogFile is the default log file location for trace agent
	defaultTraceAgentLogFile = defaultLogPath + "/trace-agent.log"
	// defaultSecurityAgentLogFile is the default log file location for security agent
	defaultSecurityAgentLogFile = defaultLogPath + "/security-agent.log"
	// defaultProcessAgentLogFile is the default log file location for process agent
	defaultProcessAgentLogFile = defaultLogPath + "/process-agent.log"
	// defaultSystemProbeLogFile is the default log file location for the system probe
	defaultSystemProbeLogFile = defaultLogPath + "/system-probe.log"
	// defaultOTelAgentLogFile is the default log file location for the otel agent
	defaultOTelAgentLogFile = defaultLogPath + "/otel-agent.log"
	// defaultHostProfilerLogFile is the default log file location for the host profiler
	defaultHostProfilerLogFile = defaultLogPath + "/host-profiler.log"
	// defaultPrivateActionRunnerLogFile is the default log file location for the private action runner
	defaultPrivateActionRunnerLogFile = defaultLogPath + "/private-action-runner.log"
	// defaultDataPlaneLogFile is the default log file used by the data-plane agent if not configured
	defaultDataPlaneLogFile = defaultLogPath + "/agent-data-plane.log"

	// Sockets

	// defaultStatsdSocket is the default dogstatsd socket path, it is empty on darwin
	defaultStatsdSocket = ""
	// defaultStatsdSocket is the default trace agent socket path, it is empty on darwin
	defaultReceiverSocket = ""
)

var (
	binaryFolder, _ = executable.Folder()
	installPath     = filepath.Join(binaryFolder, "..", "..")

	// PyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	PyChecksPath = filepath.Join(installPath, "checks.d")
	// DistPath holds the path to the folder containing distribution files
	distPath = filepath.Join(installPath, "bin", "agent", "dist")
)

// GetDistPath returns the fully qualified path to the 'dist' directory
func GetDistPath() string {
	return distPath
}

// GetInstallPath returns the fully qualified path to the datadog-agent executable
func GetInstallPath() string {
	return installPath
}

// GetDefaultRunPath returns the path to the run directory
func GetDefaultRunPath() string {
	return defaultRunPath
}

// GetDefaultConfPath returns the path to the folder containing datadog.yaml
func GetDefaultConfPath() string {
	return defaultConfPath
}

// GetDefaultConfFile returns the default location of datadog.yaml
func GetDefaultConfFile() string {
	return filepath.Join(GetDefaultConfPath(), "datadog.yaml")
}

// GetDefaultSysProbeConfFile returns the default location of system-probe.yaml
func GetDefaultSysProbeConfFile() string {
	return filepath.Join(GetDefaultConfPath(), "system-probe.yaml")
}

// GetDefaultPyChecksPath returns the path to the python checks directory
func GetDefaultPyChecksPath() string {
	return defaultPyChecksPath
}

// GetDefaultStatsdSocket returns the path to the run directory
func GetDefaultStatsdSocket() string {
	return defaultStatsdSocket
}

// GetDefaultReceiverSocket returns the path to the APM receiver Unix socket
func GetDefaultReceiverSocket() string {
	return defaultReceiverSocket
}

// GetDefaultJMXFlareDirectory returns the path to the JMX flare directory
func GetDefaultJMXFlareDirectory() string {
	return defaultJMXFlareDirectory
}

// GetDefaultCheckFlareDirectory returns the path to the check flare directory
func GetDefaultCheckFlareDirectory() string {
	return defaultCheckFlareDirectory
}

// GetDefaultSystemProbeLogFile returns the path to the system-probe log file
func GetDefaultSystemProbeLogFile() string {
	return defaultSystemProbeLogFile
}

// GetDefaultPrivateActionRunnerLogFile returns the path to the private-action-runner log file
func GetDefaultPrivateActionRunnerLogFile() string {
	return defaultPrivateActionRunnerLogFile
}

// GetDefaultHostProfilerLogFile returns the path to the host-profiler log file
func GetDefaultHostProfilerLogFile() string {
	return defaultHostProfilerLogFile
}

// GetDefaultOTelAgentLogFile returns the path to the otel-agent log file
func GetDefaultOTelAgentLogFile() string {
	return defaultOTelAgentLogFile
}

// GetDefaultProcessAgentLogFile returns the path to the process-agent log file
func GetDefaultProcessAgentLogFile() string {
	return defaultProcessAgentLogFile
}

// GetDefaultSecurityAgentLogFile returns the path to the security-agent log file
func GetDefaultSecurityAgentLogFile() string {
	return defaultSecurityAgentLogFile
}

// GetDefaultUpdaterLogFile returns the path to the updater log file
func GetDefaultUpdaterLogFile() string {
	return defaultUpdaterLogFile
}

// GetDefaultStreamlogsLogFile returns the path to the streamlogs log file
func GetDefaultStreamlogsLogFile() string {
	return defaultStreamlogsLogFile
}

// GetDefaultTraceAgentLogFile returns the path to the trace-agent log file
func GetDefaultTraceAgentLogFile() string {
	return defaultTraceAgentLogFile
}

// GetDefaultDogstatsDServiceLogFile returns the path to the legacy dogstatsd log file location
func GetDefaultDogstatsDServiceLogFile() string {
	return defaultDogstatsDServiceLogFile
}

// GetDefaultDogstatsDProtocolLogFile returns the path to the DogStatsD protocol stats log file
func GetDefaultDogstatsDProtocolLogFile() string {
	return defaultDogstatsDProtocolLogFile
}

// GetDefaultJmxLogFile returns the path to the jmxfetch log file
func GetDefaultJmxLogFile() string {
	return defaultJmxLogFile
}

// GetDefaultDCALogFile returns the path to the cluster-agent log file
func GetDefaultDCALogFile() string {
	return defaultDCALogFile
}

// GetDefaultLogPath returns the path to the agent log directory
func GetDefaultLogPath() string {
	return defaultLogPath
}

// GetDefaultLogFile returns the path to the agent log file
func GetDefaultLogFile() string {
	return defaultLogFile
}

// GetEmbeddedBinPath returns the path of the embedded binary.
func GetEmbeddedBinPath() string {
	return filepath.Join(GetInstallPath(), "embedded", "bin")
}

// GetDefaultSystemProbeAddress returns the default unix socket path to be used for connecting to the system probe
func GetDefaultSystemProbeAddress() string {
	return filepath.Join(GetInstallPath(), "run", "sysprobe.sock")
}

// GetDefaultDDAgentBin returns the default path to the core agent binary
func GetDefaultDDAgentBin() string {
	return filepath.Join(GetInstallPath(), "bin", "agent", "agent")
}

// GetDefaultDataPlaneLogFile returns the default log file used by the data-plane agent if not configured
func GetDefaultDataPlaneLogFile() string {
	return defaultDataPlaneLogFile
}
