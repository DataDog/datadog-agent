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
	ConfPath = "/usr/local/etc/datadog-agent"
	// LogFile points to the log file that will be used if not configured
	LogFile = "/var/log/datadog/agent.log"
	// DCALogFile points to the log file that will be used if not configured
	DCALogFile = "/var/log/datadog/cluster-agent.log"
	//JmxLogFile points to the jmx fetch log file that will be used if not configured
	JmxLogFile = "/var/log/datadog/jmxfetch.log"
	// CheckFlareDirectory a flare friendly location for checks to be written
	CheckFlareDirectory = "/var/log/datadog/checks/"
	// JMXFlareDirectory a flare friendly location for jmx command logs to be written
	JMXFlareDirectory = "/var/log/datadog/jmxinfo/"
	// DogstatsDLogFile points to the dogstatsd stats log file that will be used if not configured
	DogstatsDLogFile = "/var/log/datadog/dogstatsd_info/dogstatsd-stats.log"
	// StreamlogsLogFile points to the stream logs log file that will be used if not configured
	StreamlogsLogFile = "/var/log/datadog/streamlogs_info/streamlogs.log"
)

var (
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

// GetInstallPath returns the fully qualified path to the datadog-agent executable
func GetInstallPath() string {
	return _here
}
