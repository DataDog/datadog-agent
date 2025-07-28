// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultpaths

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"

	"golang.org/x/sys/windows/registry"
)

var (
	// utility variables
	_here, _ = executable.Folder()
	// PyChecksPath holds the path to the python checks from integrations-core shipped with the agent
	PyChecksPath = filepath.Join(_here, "..", "checks.d")
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
}

// GetInstallPath returns the fully qualified path to the datadog-agent executable
func GetInstallPath() string {
	// fetch the installation path from the registry
	installpath := filepath.Join(_here, "..")
	var s string
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\DataDog\Datadog Agent`, registry.QUERY_VALUE)
	if err != nil {
		log.Warnf("Failed to open registry key: %s", err)
	} else {
		defer k.Close()
		s, _, err = k.GetStringValue("InstallPath")
		if err != nil {
			log.Warnf("Installpath not found in registry: %s", err)
		}
	}
	// if unable to figure out the install path from the registry,
	// just compute it relative to the executable.
	if s == "" {
		s = installpath
	}
	return s
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
