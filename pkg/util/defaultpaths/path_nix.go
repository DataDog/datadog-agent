// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build netbsd || openbsd || solaris || dragonfly || linux

package defaultpaths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

// Default paths for Linux systems following the Filesystem Hierarchy Standard (FHS).
// To access one of these paths, use the corresponding getter function below.
const (
	// Config paths
	confPath             = "/etc/datadog-agent"
	confdPath            = "/etc/datadog-agent/conf.d"
	additionalChecksPath = "/etc/datadog-agent/checks.d"

	// Log files
	logFile                    = "/var/log/datadog/agent.log"
	dcaLogFile                 = "/var/log/datadog/cluster-agent.log"
	jmxLogFile                 = "/var/log/datadog/jmxfetch.log"
	dogstatsDProtocolLogFile   = "/var/log/datadog/dogstatsd_info/dogstatsd-stats.log"
	dogstatsDServiceLogFile    = "/var/log/datadog/dogstatsd.log"
	traceAgentLogFile          = "/var/log/datadog/trace-agent.log"
	streamlogsLogFile          = "/var/log/datadog/streamlogs_info/streamlogs.log"
	updaterLogFile             = "/var/log/datadog/updater.log"
	securityAgentLogFile       = "/var/log/datadog/security-agent.log"
	processAgentLogFile        = "/var/log/datadog/process-agent.log"
	otelAgentLogFile           = "/var/log/datadog/otel-agent.log"
	hostProfilerLogFile        = "/var/log/datadog/host-profiler.log"
	privateActionRunnerLogFile = "/var/log/datadog/private-action-runner.log"
	systemProbeLogFile         = "/var/log/datadog/system-probe.log"

	// Flare directories
	checkFlareDirectory = "/var/log/datadog/checks/"
	jmxFlareDirectory   = "/var/log/datadog/jmxinfo/"

	// Socket paths - use {InstallPath}/run for consistency with other runtime files
	defaultStatsdSocket   = "/opt/datadog-agent/run/dsd.socket"
	defaultReceiverSocket = "/opt/datadog-agent/run/apm.socket"

	// Python checks path (bundled integrations-core checks)
	pyChecksPath = "/opt/datadog-agent/checks.d"

	// Default install path fallback
	defaultInstallPath = "/opt/datadog-agent"
)

// Exported default path constants for use in BindEnvAndSetDefault and similar config registration.
// These are the raw, untransformed FHS paths. Use getter functions for runtime transformed paths.
const (
	DefaultConfPath                   = confPath
	DefaultConfdPath                  = confdPath
	DefaultAdditionalChecksPath       = additionalChecksPath
	DefaultLogFile                    = logFile
	DefaultUpdaterLogFile             = updaterLogFile
	DefaultSecurityAgentLogFile       = securityAgentLogFile
	DefaultProcessAgentLogFile        = processAgentLogFile
	DefaultOTelAgentLogFile           = otelAgentLogFile
	DefaultHostProfilerLogFile        = hostProfilerLogFile
	DefaultPrivateActionRunnerLogFile = privateActionRunnerLogFile
	DefaultStreamlogsLogFile          = streamlogsLogFile
	DefaultSystemProbeLogFile         = systemProbeLogFile
	DefaultStatsdSocket               = defaultStatsdSocket
	DefaultReceiverSocket             = defaultReceiverSocket
)

var (
	// _here is the directory containing the agent executable
	_here, _ = executable.Folder()
	// distPath holds the path to the folder containing distribution files
	// This is relative to the executable location, not the install root
	distPath = filepath.Join(_here, "dist")
	// detectedInstallPath is the install path detected from the executable location
	detectedInstallPath = detectInstallPath(_here)
)

// detectInstallPath walks up the directory tree from start looking for a .install_root marker file.
// If found, returns that directory. Otherwise returns the default install path.
func detectInstallPath(start string) string {
	if start == "" {
		return defaultInstallPath
	}
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

// Config path getters

// GetDefaultConfPath returns the path to the folder containing datadog.yaml
func GetDefaultConfPath() string {
	return CommonRootOrPath(commonRoot, confPath)
}

// GetDefaultConfdPath returns the path to the conf.d directory
func GetDefaultConfdPath() string {
	return CommonRootOrPath(commonRoot, confdPath)
}

// GetDefaultAdditionalChecksPath returns the path to the checks.d directory
func GetDefaultAdditionalChecksPath() string {
	return CommonRootOrPath(commonRoot, additionalChecksPath)
}

// Log file getters

// GetDefaultLogFile returns the path to the agent log file
func GetDefaultLogFile() string {
	return CommonRootOrPath(commonRoot, logFile)
}

// GetDefaultDCALogFile returns the path to the cluster-agent log file
func GetDefaultDCALogFile() string {
	return CommonRootOrPath(commonRoot, dcaLogFile)
}

// GetDefaultJmxLogFile returns the path to the jmxfetch log file
func GetDefaultJmxLogFile() string {
	return CommonRootOrPath(commonRoot, jmxLogFile)
}

// GetDefaultDogstatsDProtocolLogFile returns the path to the DogStatsD protocol stats log file
func GetDefaultDogstatsDProtocolLogFile() string {
	return CommonRootOrPath(commonRoot, dogstatsDProtocolLogFile)
}

// GetDefaultDogstatsDServiceLogFile returns the path to the dogstatsd service log file
func GetDefaultDogstatsDServiceLogFile() string {
	return CommonRootOrPath(commonRoot, dogstatsDServiceLogFile)
}

// GetDefaultTraceAgentLogFile returns the path to the trace-agent log file
func GetDefaultTraceAgentLogFile() string {
	return CommonRootOrPath(commonRoot, traceAgentLogFile)
}

// GetDefaultStreamlogsLogFile returns the path to the streamlogs log file
func GetDefaultStreamlogsLogFile() string {
	return CommonRootOrPath(commonRoot, streamlogsLogFile)
}

// GetDefaultUpdaterLogFile returns the path to the updater log file
func GetDefaultUpdaterLogFile() string {
	return CommonRootOrPath(commonRoot, updaterLogFile)
}

// GetDefaultSecurityAgentLogFile returns the path to the security-agent log file
func GetDefaultSecurityAgentLogFile() string {
	return CommonRootOrPath(commonRoot, securityAgentLogFile)
}

// GetDefaultProcessAgentLogFile returns the path to the process-agent log file
func GetDefaultProcessAgentLogFile() string {
	return CommonRootOrPath(commonRoot, processAgentLogFile)
}

// GetDefaultOTelAgentLogFile returns the path to the otel-agent log file
func GetDefaultOTelAgentLogFile() string {
	return CommonRootOrPath(commonRoot, otelAgentLogFile)
}

// GetDefaultHostProfilerLogFile returns the path to the host-profiler log file
func GetDefaultHostProfilerLogFile() string {
	return CommonRootOrPath(commonRoot, hostProfilerLogFile)
}

// GetDefaultPrivateActionRunnerLogFile returns the path to the private-action-runner log file
func GetDefaultPrivateActionRunnerLogFile() string {
	return CommonRootOrPath(commonRoot, privateActionRunnerLogFile)
}

// GetDefaultSystemProbeLogFile returns the path to the system-probe log file
func GetDefaultSystemProbeLogFile() string {
	return CommonRootOrPath(commonRoot, systemProbeLogFile)
}

// Flare directory getters

// GetDefaultCheckFlareDirectory returns the path to the check flare directory
func GetDefaultCheckFlareDirectory() string {
	return CommonRootOrPath(commonRoot, checkFlareDirectory)
}

// GetDefaultJMXFlareDirectory returns the path to the JMX flare directory
func GetDefaultJMXFlareDirectory() string {
	return CommonRootOrPath(commonRoot, jmxFlareDirectory)
}

// Socket path getters

// GetDefaultStatsdSocket returns the path to the dogstatsd Unix socket
func GetDefaultStatsdSocket() string {
	return filepath.Join(GetDefaultRunPath(), "dsd.socket")
}

// GetDefaultReceiverSocket returns the path to the APM receiver Unix socket
func GetDefaultReceiverSocket() string {
	return filepath.Join(GetDefaultRunPath(), "apm.socket")
}

// GetDefaultPidFilePath returns the path to the agent PID file
func GetDefaultPidFilePath() string {
	return filepath.Join(GetDefaultRunPath(), "datadog-agent.pid")
}

// GetDefaultRunPath returns the path to the directory used to store runtime files
func GetDefaultRunPath() string {
	return filepath.Join(GetInstallPath(), "run")
}

// Other path getters

// GetDefaultPyChecksPath returns the path to the python checks directory
func GetDefaultPyChecksPath() string {
	return CommonRootOrPath(commonRoot, pyChecksPath)
}

// GetDistPath returns the fully qualified path to the 'dist' directory
func GetDistPath() string {
	if _here == "" {
		// Fallback if executable.Folder() failed during init
		return CommonRootOrPath(commonRoot, "/opt/datadog-agent/bin/agent/dist")
	}
	return distPath
}

// GetInstallPath returns the install root path for the agent (e.g., /opt/datadog-agent)
func GetInstallPath() string {
	return CommonRootOrPath(commonRoot, detectedInstallPath)
}

// GetBinPath returns the directory containing the agent executable.
// This is used by code that needs to find files relative to the executable location.
func GetBinPath() string {
	if _here == "" {
		// Fallback if executable.Folder() failed during init
		return CommonRootOrPath(commonRoot, "/opt/datadog-agent/bin/agent")
	}
	return _here
}

// CommonRootOrPath will optionally transform the path to use the common root path depending
// on the configuration.
//
//	/etc/datadog-agent/** -> {root}/etc/**
//	/var/log/datadog/**   -> {root}/logs/**
//	/var/run/datadog/**   -> {root}/run/**
//	/opt/datadog-agent/** -> {root}/**
func CommonRootOrPath(root, path string) string {
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
