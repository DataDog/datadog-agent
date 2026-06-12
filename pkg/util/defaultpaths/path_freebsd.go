// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultpaths

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

// Private default path constants for reference. BindEnvAndSetDefault uses getter functions after init().
// FreeBSD follows BSD conventions: configuration files live under /usr/local/etc, while runtime
// and log files match the layout used on Linux.
const (
	// defaultConfPath points to the folder containing datadog.yaml
	defaultConfPath = "/usr/local/etc/datadog-agent"
	// defaultConfdPath points to the folder containing integration configuration files
	defaultConfdPath = "/usr/local/etc/datadog-agent/conf.d"
	// defaultAdditionalChecksPath points to the folder containing custom python integration files
	defaultAdditionalChecksPath = "/usr/local/etc/datadog-agent/checks.d"
	// defaultPyChecksPath points to the folder containing preinstalled integrations with the agent
	defaultPyChecksPath = "/opt/datadog-agent/checks.d"
	// defaultBinPath is the installation folder for agent binaries
	defaultBinPath = "/opt/datadog-agent/bin/agent"
	// defaultCheckFlareDirectory a flare friendly location for checks to be written
	defaultCheckFlareDirectory = "/var/log/datadog/checks/"
	// defaultJMXFlareDirectory a flare friendly location for jmx command logs to be written
	defaultJMXFlareDirectory = "/var/log/datadog/jmxinfo/"

	// Log files

	// defaultLogFile points to the log file that will be used if not configured
	defaultLogFile = "/var/log/datadog/agent.log"
	// defaultDCALogFile points to the log file that will be used if not configured
	defaultDCALogFile = "/var/log/datadog/cluster-agent.log"
	// defaultJmxLogFile points to the jmx fetch log file that will be used if not configured
	defaultJmxLogFile = "/var/log/datadog/jmxfetch.log"
	// defaultDogstatsDServiceLogFile points to the old datadog.conf dogstatsd_log_file location for running dogstatsd in as a standalone service
	defaultDogstatsDServiceLogFile = "/var/log/datadog/dogstatsd.log"
	// defaultDogstatsDProtocolLogFile points to the dogstatsd stats log file that will be used if not configured
	defaultDogstatsDProtocolLogFile = "/var/log/datadog/dogstatsd_info/dogstatsd-stats.log"
	// defaultStreamlogsLogFile points to the stream logs log file that will be used if not configured
	defaultStreamlogsLogFile = "/var/log/datadog/streamlogs_info/streamlogs.log"
	// defaultUpdaterLogFile is the default log file location for updater
	defaultUpdaterLogFile = "/var/log/datadog/updater.log"
	// defaultTraceAgentLogFile is the default log file location for trace agent
	defaultTraceAgentLogFile = "/var/log/datadog/trace-agent.log"
	// defaultSecurityAgentLogFile is the default log file location for security agent
	defaultSecurityAgentLogFile = "/var/log/datadog/security-agent.log"
	// defaultProcessAgentLogFile is the default log file location for process agent
	defaultProcessAgentLogFile = "/var/log/datadog/process-agent.log"
	// defaultSystemProbeLogFile is the default log file location for the system probe
	defaultSystemProbeLogFile = "/var/log/datadog/system-probe.log"
	// defaultOTelAgentLogFile is the default log file location for the otel agent
	defaultOTelAgentLogFile = "/var/log/datadog/otel-agent.log"
	// defaultHostProfilerLogFile is the default log file location for the host profiler
	defaultHostProfilerLogFile = "/var/log/datadog/host-profiler.log"
	// defaultPrivateActionRunnerLogFile is the default log file location for the private action runner
	defaultPrivateActionRunnerLogFile = "/var/log/datadog/private-action-runner.log"
	// defaultDataPlaneLogFile is the default log file used by the data-plane agent if not configured
	defaultDataPlaneLogFile = "/var/log/datadog/agent-data-plane.log"

	// Sockets

	// defaultStatsdSocket is the default dogstatsd socket path
	defaultStatsdSocket = "/var/run/datadog/dsd.socket"
	// defaultReceiverSocket is the default trace-agent receiver socket path
	defaultReceiverSocket = "/var/run/datadog/apm.socket"
)

var (
	// defaultInstallPath is the default install path for the agent.
	defaultInstallPath = "/opt/datadog-agent"

	// runPath is the default run path for the agent.
	runPath = ""

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

// GetInstallPath returns the fully qualified path to the datadog-agent installation directory
func GetInstallPath() string {
	return getInstallPathFromExecutable(_here)
}

// GetDefaultRunPath returns the path to the run directory
func GetDefaultRunPath() string {
	// runPath might be set by a ldflag -X target, if so use that
	if runPath != "" {
		return runPath
	}
	return filepath.Join(GetInstallPath(), "run")
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

// GetDefaultConfdPath returns the path to the conf.d directory
func GetDefaultConfdPath() string {
	return defaultConfdPath
}

// GetDefaultAdditionalChecksPath returns the path to the checks.d directory
func GetDefaultAdditionalChecksPath() string {
	return defaultAdditionalChecksPath
}

// GetDefaultPyChecksPath returns the path to the python checks directory
func GetDefaultPyChecksPath() string {
	return defaultPyChecksPath
}

// GetDefaultPidFilePath returns the path to the agent PID file
func GetDefaultPidFilePath() string {
	return filepath.Join(GetDefaultRunPath(), "datadog-agent.pid")
}

// GetBinPath returns the directory containing the agent executable.
// This is used by code that needs to find files relative to the executable location.
func GetBinPath() string {
	return defaultBinPath
}

// GetDefaultStatsdSocket returns the path to the default DogStatsD Unix socket
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

// GetDefaultLogFile returns the path to the agent log file
func GetDefaultLogFile() string {
	return defaultLogFile
}

// getInstallPathFromExecutable walks up the directory chain from start in search of a .install_root file.
// That directory becomes the install path. If not found, returns defaultInstallPath.
func getInstallPathFromExecutable(start string) string {
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
	return defaultInstallPath
}

// GetEmbeddedBinPath returns the path of the embedded binary.
func GetEmbeddedBinPath() string {
	return filepath.Join(GetInstallPath(), "embedded", "bin")
}

// GetDefaultSystemProbeAddress returns the default unix socket path to be used for connecting to the system probe
func GetDefaultSystemProbeAddress() string {
	return filepath.Join(GetInstallPath(), "run/sysprobe.sock")
}

// GetDefaultDDAgentBin returns the default path to the core agent binary
func GetDefaultDDAgentBin() string {
	return filepath.Join(GetInstallPath(), "bin/agent/agent")
}

// GetDefaultDataPlaneLogFile returns the default log file used by the data-plane agent if not configured
func GetDefaultDataPlaneLogFile() string {
	return defaultDataPlaneLogFile
}
