// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultpaths

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"golang.org/x/sys/windows/registry"
)

// NOTE: Do NOT calculate paths relative to the executable.
//
//	The agent executables are not all installed in the same path and this can
//	lead to incorrect path calculations when this package is imported by
//	other executables.
//
// defaultInstallPath is the default install path for the Agent. The install path is
// customizable at install time, so we update it from the registry in init().
const defaultInstallPath = `C:\Program Files\Datadog\Datadog Agent`

var (
	installPath string
	// pyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	pyChecksPath string
	distPath     string
)

// Exported default path constants for use in BindEnvAndSetDefault and similar config registration.
// These are the static default values before init() may update them based on registry.
// For runtime paths that reflect registry customization, use getter functions instead.
const (
	DefaultConfPath                   = "c:\\programdata\\datadog"
	DefaultConfdPath                  = "c:\\programdata\\datadog\\conf.d"
	DefaultAdditionalChecksPath       = "c:\\programdata\\datadog\\checks.d"
	DefaultLogFile                    = "c:\\programdata\\datadog\\logs\\agent.log"
	DefaultUpdaterLogFile             = "c:\\programdata\\datadog\\logs\\updater.log"
	DefaultSecurityAgentLogFile       = "c:\\programdata\\datadog\\logs\\security-agent.log"
	DefaultProcessAgentLogFile        = "c:\\programdata\\datadog\\logs\\process-agent.log"
	DefaultOTelAgentLogFile           = "c:\\programdata\\datadog\\logs\\otel-agent.log"
	DefaultHostProfilerLogFile        = "c:\\programdata\\datadog\\logs\\host-profiler.log"
	DefaultPrivateActionRunnerLogFile = "c:\\programdata\\datadog\\logs\\private-action-runner.log"
	DefaultStreamlogsLogFile          = "c:\\programdata\\datadog\\logs\\streamlogs_info\\streamlogs.log"
	DefaultSystemProbeLogFile         = "c:\\programdata\\datadog\\logs\\system-probe.log"
	DefaultStatsdSocket               = ""
	DefaultReceiverSocket             = ""
	DefaultRunPath                    = "c:\\programdata\\datadog\\run"
)

// Default paths for Windows systems.
// These may be updated by init() based on the registry and ProgramData location.
var (
	// Config paths
	confPath             = DefaultConfPath
	confdPath            = DefaultConfdPath
	additionalChecksPath = DefaultAdditionalChecksPath

	// Log files
	logFile                    = DefaultLogFile
	dcaLogFile                 = "c:\\programdata\\datadog\\logs\\cluster-agent.log"
	jmxLogFile                 = "c:\\programdata\\datadog\\logs\\jmxfetch.log"
	dogstatsDProtocolLogFile   = "c:\\programdata\\datadog\\logs\\dogstatsd_info\\dogstatsd-stats.log"
	dogstatsDServiceLogFile    = "c:\\programdata\\datadog\\logs\\dogstatsd.log"
	traceAgentLogFile          = "c:\\programdata\\datadog\\logs\\trace-agent.log"
	streamlogsLogFile          = DefaultStreamlogsLogFile
	updaterLogFile             = DefaultUpdaterLogFile
	securityAgentLogFile       = DefaultSecurityAgentLogFile
	processAgentLogFile        = DefaultProcessAgentLogFile
	otelAgentLogFile           = DefaultOTelAgentLogFile
	hostProfilerLogFile        = DefaultHostProfilerLogFile
	privateActionRunnerLogFile = DefaultPrivateActionRunnerLogFile
	systemProbeLogFile         = DefaultSystemProbeLogFile

	// Flare directories
	checkFlareDirectory = "c:\\programdata\\datadog\\logs\\checks\\"
	jmxFlareDirectory   = "c:\\programdata\\datadog\\logs\\jmxinfo\\"

	// Socket paths (empty on Windows by default - Windows uses named pipes)
	statsdSocket   = DefaultStatsdSocket
	receiverSocket = DefaultReceiverSocket

	// Run path
	runPath = DefaultRunPath

	// PID file path
	pidFilePath = "c:\\programdata\\datadog\\datadog-agent.pid"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		confPath = pd
		confdPath = filepath.Join(pd, "conf.d")
		additionalChecksPath = filepath.Join(pd, "checks.d")
		logFile = filepath.Join(pd, "logs", "agent.log")
		dcaLogFile = filepath.Join(pd, "logs", "cluster-agent.log")
		dogstatsDProtocolLogFile = filepath.Join(pd, "logs", "dogstatsd_info", "dogstatsd-stats.log")
		dogstatsDServiceLogFile = filepath.Join(pd, "logs", "dogstatsd.log")
		traceAgentLogFile = filepath.Join(pd, "logs", "trace-agent.log")
		streamlogsLogFile = filepath.Join(pd, "logs", "streamlogs_info", "streamlogs.log")
		updaterLogFile = filepath.Join(pd, "logs", "updater.log")
		securityAgentLogFile = filepath.Join(pd, "logs", "security-agent.log")
		processAgentLogFile = filepath.Join(pd, "logs", "process-agent.log")
		otelAgentLogFile = filepath.Join(pd, "logs", "otel-agent.log")
		hostProfilerLogFile = filepath.Join(pd, "logs", "host-profiler.log")
		privateActionRunnerLogFile = filepath.Join(pd, "logs", "private-action-runner.log")
		systemProbeLogFile = filepath.Join(pd, "logs", "system-probe.log")
		checkFlareDirectory = filepath.Join(pd, "logs", "checks") + "\\"
		jmxFlareDirectory = filepath.Join(pd, "logs", "jmxinfo") + "\\"
		runPath = filepath.Join(pd, "run")
		pidFilePath = filepath.Join(pd, "datadog-agent.pid")
	}
	installPath = fetchInstallPath()
	pyChecksPath = filepath.Join(installPath, "checks.d")
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
	return dogstatsDServiceLogFile
}

// GetDefaultTraceAgentLogFile returns the path to the trace-agent log file
func GetDefaultTraceAgentLogFile() string {
	return traceAgentLogFile
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

// GetDefaultStatsdSocket returns the path to the dogstatsd Unix socket (empty on Windows)
func GetDefaultStatsdSocket() string {
	return statsdSocket
}

// GetDefaultReceiverSocket returns the path to the APM receiver Unix socket (empty on Windows)
func GetDefaultReceiverSocket() string {
	return receiverSocket
}

// GetDefaultPidFilePath returns the path to the agent PID file
func GetDefaultPidFilePath() string {
	return pidFilePath
}

// GetDefaultRunPath returns the path to the run directory
func GetDefaultRunPath() string {
	return runPath
}

// Other path getters

// GetDefaultPyChecksPath returns the path to the python checks directory
func GetDefaultPyChecksPath() string {
	return pyChecksPath
}

// GetInstallPath returns the fully qualified path to the datadog-agent installation directory
//
// default: `C:\Program Files\Datadog\Datadog Agent`
func GetInstallPath() string {
	return installPath
}

func fetchInstallPath() string {
	// fetch the installation path from the registry
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\DataDog\Datadog Agent`, registry.QUERY_VALUE)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Failed to open registry key: %s", err)
	} else {
		defer k.Close()
		s, _, err := k.GetStringValue("InstallPath")
		if err != nil {
			log.Warnf("Installpath not found in registry: %s", err)
		} else if s != "" {
			return s
		}
	}
	// return default path
	return defaultInstallPath
}

// GetDistPath returns the fully qualified path to the 'dist' directory
func GetDistPath() string {
	if len(distPath) == 0 {
		var s string
		if s = GetInstallPath(); s == "" {
			return ""
		}
		distPath = filepath.Join(s, `bin/agent/dist`)
	}
	return distPath
}

// CommonRootOrPath is not supported on Windows currently
func CommonRootOrPath(_, path string) string {
	return path
}
