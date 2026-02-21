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
	// PyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	PyChecksPath string
	distPath     string
)

var (
	// ConfPath points to the folder containing datadog.yaml
	ConfPath = "c:\\programdata\\datadog"
	// LogFile points to the log file that will be used if not configured
	LogFile = "c:\\programdata\\datadog\\logs\\agent.log"
	// DCALogFile points to the log file that will be used if not configured
	DCALogFile = "c:\\programdata\\datadog\\logs\\cluster-agent.log"
	//JmxLogFile points to the jmx fetch log file that will be used if not configured
	JmxLogFile = "c:\\programdata\\datadog\\logs\\jmxfetch.log"
	// CheckFlareDirectory a flare friendly location for checks to be written
	CheckFlareDirectory = "c:\\programdata\\datadog\\logs\\checks\\"
	// JMXFlareDirectory a flare friendly location for jmx command logs to be written
	JMXFlareDirectory = "c:\\programdata\\datadog\\logs\\jmxinfo\\"
	// DogstatsDLogFile points to the dogstatsd stats log file that will be used if not configured
	DogstatsDLogFile = "c:\\programdata\\datadog\\logs\\dogstatsd_info\\dogstatsd-stats.log"
	// StreamlogsLogFile points to the stream logs log file that will be used if not configured
	StreamlogsLogFile = "c:\\programdata\\datadog\\logs\\streamlogs_info\\streamlogs.log"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		ConfPath = pd
		LogFile = filepath.Join(pd, "logs", "agent.log")
		DCALogFile = filepath.Join(pd, "logs", "cluster-agent.log")
		DogstatsDLogFile = filepath.Join(pd, "logs", "dogstatsd_info", "dogstatsd-stats.log")
		StreamlogsLogFile = filepath.Join(pd, "logs", "streamlogs_info", "streamlogs.log")
	}
	installPath = fetchInstallPath()
	PyChecksPath = filepath.Join(installPath, "checks.d")
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

// GetDefaultConfPath returns the fully qualified directory path where the agent looks for the datadog.yaml config
func GetDefaultConfPath() string {
	return ConfPath
}
