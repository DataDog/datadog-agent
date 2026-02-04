// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultpaths

import (
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

// Default paths for macOS systems.
// To access one of these paths, use the corresponding getter function below.
const (
	// Config paths
	confPath             = "/opt/datadog-agent/etc"
	confdPath            = "/opt/datadog-agent/etc/conf.d"
	additionalChecksPath = "/opt/datadog-agent/etc/checks.d"

	// Log files
	logFile                    = "/opt/datadog-agent/logs/agent.log"
	dcaLogFile                 = "/opt/datadog-agent/logs/cluster-agent.log"
	jmxLogFile                 = "/opt/datadog-agent/logs/jmxfetch.log"
	dogstatsDProtocolLogFile   = "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log"
	dogstatsDServiceLogFile    = "/opt/datadog-agent/logs/dogstatsd.log"
	traceAgentLogFile          = "/opt/datadog-agent/logs/trace-agent.log"
	streamlogsLogFile          = "/opt/datadog-agent/logs/streamlogs_info/streamlogs.log"
	updaterLogFile             = "/opt/datadog-agent/logs/updater.log"
	securityAgentLogFile       = "/opt/datadog-agent/logs/security-agent.log"
	processAgentLogFile        = "/opt/datadog-agent/logs/process-agent.log"
	otelAgentLogFile           = "/opt/datadog-agent/logs/otel-agent.log"
	hostProfilerLogFile        = "/opt/datadog-agent/logs/host-profiler.log"
	privateActionRunnerLogFile = "/opt/datadog-agent/logs/private-action-runner.log"
	systemProbeLogFile         = "/opt/datadog-agent/logs/system-probe.log"

	// Flare directories
	checkFlareDirectory = "/opt/datadog-agent/logs/checks/"
	jmxFlareDirectory   = "/opt/datadog-agent/logs/jmxinfo/"

	// Socket paths (empty on Darwin by default)
	statsdSocket   = ""
	receiverSocket = ""

	// Run path
	runPath = "/opt/datadog-agent/run"

	// PID file path
	pidFilePath = "/opt/datadog-agent/run/datadog-agent.pid"

	// Python checks path (bundled integrations-core checks)
	pyChecksPath = "/opt/datadog-agent/checks.d"

	// Default install path
	defaultInstallPath = "/opt/datadog-agent"
)

// Exported default path constants for use in BindEnvAndSetDefault and similar config registration.
// These are the raw, untransformed paths. Use getter functions for runtime transformed paths.
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
	DefaultStatsdSocket               = statsdSocket
	DefaultReceiverSocket             = receiverSocket
	DefaultRunPath                    = runPath
)

var (
	// _here is the directory containing the agent executable
	_here, _ = executable.Folder()
	// distPath holds the path to the folder containing distribution files
	// This is relative to the executable location, not the install root
	distPath = filepath.Join(_here, "dist")
)

// Config path getters

// GetConfPath returns the path to the folder containing datadog.yaml
func GetConfPath() string {
	return CommonRootOrPath(commonRoot, confPath)
}

// GetConfdPath returns the path to the conf.d directory
func GetConfdPath() string {
	return CommonRootOrPath(commonRoot, confdPath)
}

// GetAdditionalChecksPath returns the path to the checks.d directory
func GetAdditionalChecksPath() string {
	return CommonRootOrPath(commonRoot, additionalChecksPath)
}

// Log file getters

// GetLogFile returns the path to the agent log file
func GetLogFile() string {
	return CommonRootOrPath(commonRoot, logFile)
}

// GetDCALogFile returns the path to the cluster-agent log file
func GetDCALogFile() string {
	return CommonRootOrPath(commonRoot, dcaLogFile)
}

// GetJmxLogFile returns the path to the jmxfetch log file
func GetJmxLogFile() string {
	return CommonRootOrPath(commonRoot, jmxLogFile)
}

// GetDogstatsDProtocolLogFile returns the path to the DogStatsD protocol stats log file
func GetDogstatsDProtocolLogFile() string {
	return CommonRootOrPath(commonRoot, dogstatsDProtocolLogFile)
}

// GetDogstatsDServiceLogFile returns the path to the dogstatsd service log file
func GetDogstatsDServiceLogFile() string {
	return CommonRootOrPath(commonRoot, dogstatsDServiceLogFile)
}

// GetTraceAgentLogFile returns the path to the trace-agent log file
func GetTraceAgentLogFile() string {
	return CommonRootOrPath(commonRoot, traceAgentLogFile)
}

// GetStreamlogsLogFile returns the path to the streamlogs log file
func GetStreamlogsLogFile() string {
	return CommonRootOrPath(commonRoot, streamlogsLogFile)
}

// GetUpdaterLogFile returns the path to the updater log file
func GetUpdaterLogFile() string {
	return CommonRootOrPath(commonRoot, updaterLogFile)
}

// GetSecurityAgentLogFile returns the path to the security-agent log file
func GetSecurityAgentLogFile() string {
	return CommonRootOrPath(commonRoot, securityAgentLogFile)
}

// GetProcessAgentLogFile returns the path to the process-agent log file
func GetProcessAgentLogFile() string {
	return CommonRootOrPath(commonRoot, processAgentLogFile)
}

// GetOTelAgentLogFile returns the path to the otel-agent log file
func GetOTelAgentLogFile() string {
	return CommonRootOrPath(commonRoot, otelAgentLogFile)
}

// GetHostProfilerLogFile returns the path to the host-profiler log file
func GetHostProfilerLogFile() string {
	return CommonRootOrPath(commonRoot, hostProfilerLogFile)
}

// GetPrivateActionRunnerLogFile returns the path to the private-action-runner log file
func GetPrivateActionRunnerLogFile() string {
	return CommonRootOrPath(commonRoot, privateActionRunnerLogFile)
}

// GetSystemProbeLogFile returns the path to the system-probe log file
func GetSystemProbeLogFile() string {
	return CommonRootOrPath(commonRoot, systemProbeLogFile)
}

// Flare directory getters

// GetCheckFlareDirectory returns the path to the check flare directory
func GetCheckFlareDirectory() string {
	return CommonRootOrPath(commonRoot, checkFlareDirectory)
}

// GetJMXFlareDirectory returns the path to the JMX flare directory
func GetJMXFlareDirectory() string {
	return CommonRootOrPath(commonRoot, jmxFlareDirectory)
}

// Socket path getters

// GetStatsdSocket returns the path to the dogstatsd Unix socket (empty on Darwin)
func GetStatsdSocket() string {
	return statsdSocket
}

// GetReceiverSocket returns the path to the APM receiver Unix socket (empty on Darwin)
func GetReceiverSocket() string {
	return receiverSocket
}

// GetPidFilePath returns the path to the agent PID file
func GetPidFilePath() string {
	return CommonRootOrPath(commonRoot, pidFilePath)
}

// GetRunPath returns the path to the run directory
func GetRunPath() string {
	return CommonRootOrPath(commonRoot, runPath)
}

// Other path getters

// GetPyChecksPath returns the path to the python checks directory
func GetPyChecksPath() string {
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

// GetInstallPath returns the install root path for the agent (e.g., /opt/datadog-agent).
// When commonRoot is set, this returns the common root path.
func GetInstallPath() string {
	return CommonRootOrPath(commonRoot, defaultInstallPath)
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

// CommonRootOrPath will replace the known darwin filesystem paths with a common root
//
//	/opt/datadog-agent/etc/** -> {root}/etc/**
//	/opt/datadog-agent/logs/** -> {root}/logs/**
//	/opt/datadog-agent/run/** -> {root}/run/**
//	/opt/datadog-agent/** -> {root}/**
//	/opt/datadog-agent -> {root}
func CommonRootOrPath(root, path string) string {
	if root == "" {
		return path
	}

	switch {
	case strings.HasPrefix(path, "/opt/datadog-agent/etc/"):
		rest := strings.TrimPrefix(path, "/opt/datadog-agent/etc/")
		return filepath.Join(root, "etc", rest)
	case path == "/opt/datadog-agent/etc":
		return filepath.Join(root, "etc")
	case strings.HasPrefix(path, "/opt/datadog-agent/logs/"):
		rest := strings.TrimPrefix(path, "/opt/datadog-agent/logs/")
		return filepath.Join(root, "logs", rest)
	case path == "/opt/datadog-agent/logs":
		return filepath.Join(root, "logs")
	case strings.HasPrefix(path, "/opt/datadog-agent/run/"):
		rest := strings.TrimPrefix(path, "/opt/datadog-agent/run/")
		return filepath.Join(root, "run", rest)
	case path == "/opt/datadog-agent/run":
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
