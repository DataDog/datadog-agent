// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build netbsd || openbsd || solaris || dragonfly || linux

package defaultpaths

import (
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
	logFile                  = "/var/log/datadog/agent.log"
	dcaLogFile               = "/var/log/datadog/cluster-agent.log"
	jmxLogFile               = "/var/log/datadog/jmxfetch.log"
	dogstatsDProtocolLogFile = "/var/log/datadog/dogstatsd_info/dogstatsd-stats.log"
	dogstatsDServiceLogFile  = "/var/log/datadog/dogstatsd.log"
	traceAgentLogFile        = "/var/log/datadog/trace-agent.log"
	streamlogsLogFile        = "/var/log/datadog/streamlogs_info/streamlogs.log"
	updaterLogFile           = "/var/log/datadog/updater.log"
	securityAgentLogFile     = "/var/log/datadog/security-agent.log"
	processAgentLogFile      = "/var/log/datadog/process-agent.log"
	otelAgentLogFile         = "/var/log/datadog/otel-agent.log"
	hostProfilerLogFile      = "/var/log/datadog/host-profiler.log"
	systemProbeLogFile       = "/var/log/datadog/system-probe.log"

	// Flare directories
	checkFlareDirectory = "/var/log/datadog/checks/"
	jmxFlareDirectory   = "/var/log/datadog/jmxinfo/"

	// Socket paths (in /var/run/datadog, needs transformation)
	statsdSocket   = "/var/run/datadog/dsd.socket"
	receiverSocket = "/var/run/datadog/apm.socket"

	// Run path
	runPath = "/var/run/datadog"

	// PID file path
	pidFilePath = "/var/run/datadog/datadog-agent.pid"
)

// Exported default path constants for use in BindEnvAndSetDefault and similar config registration.
// These are the raw, untransformed FHS paths. Use getter functions for runtime transformed paths.
const (
	DefaultConfPath             = confPath
	DefaultConfdPath            = confdPath
	DefaultAdditionalChecksPath = additionalChecksPath
	DefaultLogFile              = logFile
	DefaultUpdaterLogFile       = updaterLogFile
	DefaultSecurityAgentLogFile = securityAgentLogFile
	DefaultProcessAgentLogFile  = processAgentLogFile
	DefaultOTelAgentLogFile     = otelAgentLogFile
	DefaultHostProfilerLogFile  = hostProfilerLogFile
	DefaultStreamlogsLogFile    = streamlogsLogFile
	DefaultSystemProbeLogFile   = systemProbeLogFile
	DefaultStatsdSocket         = statsdSocket
	DefaultReceiverSocket       = receiverSocket
	DefaultRunPath              = runPath
)

var (
	// utility variables
	_here, _ = executable.Folder()
	// pyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	pyChecksPath = filepath.Join(_here, "..", "..", "checks.d")
	// distPath holds the path to the folder containing distribution files
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

// GetStatsdSocket returns the path to the dogstatsd Unix socket
func GetStatsdSocket() string {
	return CommonRootOrPath(commonRoot, statsdSocket)
}

// GetReceiverSocket returns the path to the APM receiver Unix socket
func GetReceiverSocket() string {
	return CommonRootOrPath(commonRoot, receiverSocket)
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
	return pyChecksPath
}

// GetDistPath returns the fully qualified path to the 'dist' directory
func GetDistPath() string {
	return distPath
}

// GetInstallPath returns the fully qualified path to the datadog-agent executable
func GetInstallPath() string {
	return _here
}

// CommonRootOrPath will optionally transform the path to use the common root path depending
// on the configuration.
//
//	/etc/datadog-agent/** -> {root}/etc/**
//	/var/log/datadog/** -> {root}/logs/**
//	/var/run/datadog/** -> {root}/run/**
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
	default:
		return path
	}
}
