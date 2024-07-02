// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package path

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

const (
	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "/opt/datadog-agent/etc"
	// DefaultLogFile points to the log file that will be used if not configured
	DefaultLogFile = "/opt/datadog-agent/logs/agent.log"
	// DefaultDCALogFile points to the log file that will be used if not configured
	DefaultDCALogFile = "/opt/datadog-agent/logs/cluster-agent.log"
	//DefaultJmxLogFile points to the jmx fetch log file that will be used if not configured
	DefaultJmxLogFile = "/opt/datadog-agent/logs/jmxfetch.log"
	// DefaultCheckFlareDirectory a flare friendly location for checks to be written
	DefaultCheckFlareDirectory = "/opt/datadog-agent/logs/checks/"
	// DefaultJMXFlareDirectory a flare friendly location for jmx command logs to be written
	DefaultJMXFlareDirectory = "/opt/datadog-agent/logs/jmxinfo/"
	//DefaultDogstatsDLogFile points to the dogstatsd stats log file that will be used if not configured
	DefaultDogstatsDLogFile = "/opt/datadog-agent/logs/dogstatsd_info/dogstatsd-stats.log"
	//DefaultStreamlogsLogFile points to the stream logs log file that will be used if not configured
	DefaultStreamlogsLogFile = "/opt/datadog-agent/logs/streamlogs_info/streamlogs.log"
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
