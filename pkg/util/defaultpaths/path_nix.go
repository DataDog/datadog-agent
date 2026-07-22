// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build netbsd || openbsd || solaris || dragonfly || linux || aix

package defaultpaths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

// Private default path constants for reference. BindEnvAndSetDefault uses getter functions after init().
// These are the raw, untransformed paths. Use getter functions for correct runtime transformed paths.
const (
	// defaultCommonRoot is the default path used when DD_COMMON_ROOT is set but empty
	defaultCommonRoot = "/opt/datadog-agent"
	// defaultConfPath points to the folder containing datadog.yaml
	defaultConfPath = "/etc/datadog-agent"
	//defaultLogPath points to the default logs folder
	defaultLogPath = "/var/log/datadog"

	// defaultPyChecksPath points to the folder containing preinstalled integrations with the agent
	defaultPyChecksPath = "/opt/datadog-agent/checks.d"

	// Log files

	// LogFile points to the log file that will be used if not configured
	defaultLogFile = defaultLogPath + "/agent.log"
	// DCALogFile points to the log file that will be used if not configured
	defaultDCALogFile = defaultLogPath + "/cluster-agent.log"
	// defaultJmxLogFile points to the jmx fetch log file that will be used if not configured
	defaultJmxLogFile = defaultLogPath + "/jmxfetch.log"
	// defaultDogstatsDServiceLogFile points to the old datadog.conf dogstatsd_log_file location for running dogstatsd in as a standalone service
	defaultDogstatsDServiceLogFile = defaultLogPath + "/dogstatsd.log"
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

	// CheckFlareDirectory a flare friendly location for checks to be written
	defaultCheckFlareDirectory = defaultLogPath + "/checks/"
	// JMXFlareDirectory a flare friendly location for jmx command logs to be written
	defaultJMXFlareDirectory = defaultLogPath + "/jmxinfo/"

	// Sockets

	// defaultStatsdSocket is the default dogstatsd socket path, it is at /var/run/datadog for historical reasons
	defaultStatsdSocket = "/var/run/datadog/dsd.socket"
	// defaultStatsdSocket is the default trace agent socket path, it is at /var/run/datadog for historical reasons
	defaultReceiverSocket = "/var/run/datadog/apm.socket"
)

var (
	// defaultInstallPath is the default install path for the agent
	// It might be overridden at build time via ldflags so it is a var instead of const
	defaultInstallPath = "/opt/datadog-agent"

	// runPath is the default run path for the agent
	// It might be overridden at build time via ldflags so it is a var instead of const
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
	return commonRootOrPath(commonRoot, distPath)
}

// GetInstallPath returns the fully qualified path to the datadog-agent executable
func GetInstallPath() string {
	return getInstallPathFromExecutable(_here)
}

// GetDefaultRunPath returns the path to the run directory
func GetDefaultRunPath() string {
	// runPath might be set by a ldflag -X target, if so use that
	if runPath != "" {
		return runPath
	}
	return commonRootOrPath(commonRoot, filepath.Join(GetInstallPath(), "run"))
}

// GetDefaultConfPath returns the path to the folder containing datadog.yaml
func GetDefaultConfPath() string {
	return commonRootOrPath(commonRoot, defaultConfPath)
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
	return commonRootOrPath(commonRoot, defaultPyChecksPath)
}

// GetDefaultStatsdSocket returns the path to the run directory
func GetDefaultStatsdSocket() string {
	return commonRootOrPath(commonRoot, defaultStatsdSocket)
}

// GetDefaultReceiverSocket returns the path to the APM receiver Unix socket
func GetDefaultReceiverSocket() string {
	return commonRootOrPath(commonRoot, defaultReceiverSocket)
}

// GetDefaultJMXFlareDirectory returns the path to the JMX flare directory
func GetDefaultJMXFlareDirectory() string {
	return commonRootOrPath(commonRoot, defaultJMXFlareDirectory)
}

// GetDefaultCheckFlareDirectory returns the path to the check flare directory
func GetDefaultCheckFlareDirectory() string {
	return commonRootOrPath(commonRoot, defaultCheckFlareDirectory)
}

// GetDefaultSystemProbeLogFile returns the path to the system-probe log file
func GetDefaultSystemProbeLogFile() string {
	return commonRootOrPath(commonRoot, defaultSystemProbeLogFile)
}

// GetDefaultPrivateActionRunnerLogFile returns the path to the private-action-runner log file
func GetDefaultPrivateActionRunnerLogFile() string {
	return commonRootOrPath(commonRoot, defaultPrivateActionRunnerLogFile)
}

// GetDefaultHostProfilerLogFile returns the path to the host-profiler log file
func GetDefaultHostProfilerLogFile() string {
	return commonRootOrPath(commonRoot, defaultHostProfilerLogFile)
}

// GetDefaultOTelAgentLogFile returns the path to the otel-agent log file
func GetDefaultOTelAgentLogFile() string {
	return commonRootOrPath(commonRoot, defaultOTelAgentLogFile)
}

// GetDefaultProcessAgentLogFile returns the path to the process-agent log file
func GetDefaultProcessAgentLogFile() string {
	return commonRootOrPath(commonRoot, defaultProcessAgentLogFile)
}

// GetDefaultSecurityAgentLogFile returns the path to the security-agent log file
func GetDefaultSecurityAgentLogFile() string {
	return commonRootOrPath(commonRoot, defaultSecurityAgentLogFile)
}

// GetDefaultUpdaterLogFile returns the path to the updater log file
func GetDefaultUpdaterLogFile() string {
	return commonRootOrPath(commonRoot, defaultUpdaterLogFile)
}

// GetDefaultStreamlogsLogFile returns the path to the streamlogs log file
func GetDefaultStreamlogsLogFile() string {
	return commonRootOrPath(commonRoot, defaultStreamlogsLogFile)
}

// GetDefaultTraceAgentLogFile returns the path to the trace-agent log file
func GetDefaultTraceAgentLogFile() string {
	return commonRootOrPath(commonRoot, defaultTraceAgentLogFile)
}

// GetDefaultDogstatsDServiceLogFile returns the path to the legacy dogstatsd log file location
func GetDefaultDogstatsDServiceLogFile() string {
	return commonRootOrPath(commonRoot, defaultDogstatsDServiceLogFile)
}

// GetDefaultDogstatsDProtocolLogFile returns the path to the DogStatsD protocol stats log file
func GetDefaultDogstatsDProtocolLogFile() string {
	return commonRootOrPath(commonRoot, defaultDogstatsDProtocolLogFile)
}

// GetDefaultJmxLogFile returns the path to the jmxfetch log file
func GetDefaultJmxLogFile() string {
	return commonRootOrPath(commonRoot, defaultJmxLogFile)
}

// GetDefaultDCALogFile returns the path to the cluster-agent log file
func GetDefaultDCALogFile() string {
	return commonRootOrPath(commonRoot, defaultDCALogFile)
}

// GetDefaultLogPath returns the path to the agent log directory
func GetDefaultLogPath() string {
	return commonRootOrPath(commonRoot, defaultLogPath)
}

// GetDefaultLogFile returns the path to the agent log file
func GetDefaultLogFile() string {
	return commonRootOrPath(commonRoot, defaultLogFile)
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
	return defaultInstallPath // Fallback to the default install path
}

// GetEmbeddedBinPath returns the path of the embedded binary.
func GetEmbeddedBinPath() string {
	return commonRootOrPath(commonRoot, filepath.Join(GetInstallPath(), "embedded", "bin"))
}

// GetDefaultSystemProbeAddress returns the default unix socket path to be used for connecting to the system probe
func GetDefaultSystemProbeAddress() string {
	return commonRootOrPath(commonRoot, filepath.Join(GetInstallPath(), "run/sysprobe.sock"))
}

// GetDefaultDDAgentBin returns the default path to the core agent binary
func GetDefaultDDAgentBin() string {
	return commonRootOrPath(commonRoot, filepath.Join(GetInstallPath(), "bin/agent/agent"))
}

// GetDefaultDataPlaneLogFile returns the default log file used by the data-plane agent if not configured
func GetDefaultDataPlaneLogFile() string {
	return commonRootOrPath(commonRoot, defaultDataPlaneLogFile)
}

// commonRootOrPath will optionally transform the path to use the common root path provided
//
//	/etc/datadog-agent/** -> {root}/etc/**
//	/var/log/datadog/**   -> {root}/logs/**
//	/var/run/datadog/**   -> {root}/run/**
//	/opt/datadog-agent/** -> {root}/**
func commonRootOrPath(root, path string) string {
	if root == "" {
		return path
	}

	switch {
	case strings.HasPrefix(path, "/var/log/datadog/"):
		rest := strings.TrimPrefix(path, "/var/log/datadog/")
		return filepath.Join(root, "logs", rest)
	case path == "/var/log/datadog":
		return filepath.Join(root, "logs")
	case strings.HasPrefix(path, "/etc/datadog-agent/"):
		rest := strings.TrimPrefix(path, "/etc/datadog-agent/")
		return filepath.Join(root, "etc", rest)
	case path == "/etc/datadog-agent":
		return filepath.Join(root, "etc")
	case strings.HasPrefix(path, "/var/run/datadog/"):
		rest := strings.TrimPrefix(path, "/var/run/datadog/")
		return filepath.Join(root, "run", rest)
	case path == "/var/run/datadog":
		return filepath.Join(root, "run")
	case strings.HasPrefix(path, "/opt/datadog-agent/"):
		rest := strings.TrimPrefix(path, "/opt/datadog-agent/")
		return filepath.Join(root, rest)
	case path == "/opt/datadog-agent":
		return root
	default:
		return path
	}
}
