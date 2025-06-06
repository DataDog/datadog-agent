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
	// ConfPath points to the folder containing datadog.yaml
	ConfPath = "/opt/datadog-agent/etc"
	// LogFile points to the log file that will be used if not configured
	LogFile = "/opt/datadog-agent/logs/agent.log"
	// DCALogFile points to the log file that will be used if not configured
	DCALogFile = "/opt/datadog-agent/logs/cluster-agent.log"
	//JmxLogFile points to the jmx fetch log file that will be used if not configured
	JmxLogFile = "/opt/datadog-agent/logs/jmxfetch.log"
	// CheckFlareDirectory a flare friendly location for checks to be written
	CheckFlareDirectory = "/opt/datadog-agent/logs/checks/"
	// JMXFlareDirectory a flare friendly location for jmx command logs to be written
	JMXFlareDirectory = "/opt/datadog-agent/logs/jmxinfo/"
	// DogstatsDLogFile points to the dogstatsd stats log file that will be used if not configured
	DogstatsDLogFile = "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log"
	// StreamlogsLogFile points to the stream logs log file that will be used if not configured
	StreamlogsLogFile = "/opt/datadog-agent/logs/streamlogs_info/streamlogs.log"
)

var (
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
	return _here
}
