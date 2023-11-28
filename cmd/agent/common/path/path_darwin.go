// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package path

import (
	"path"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = path.Join(version.AgentPath, "etc")
	// DefaultLogFile points to the log file that will be used if not configured
	DefaultLogFile = path.Join(version.AgentPath, "logs/agent.log")
	// DefaultDCALogFile points to the log file that will be used if not configured
	DefaultDCALogFile = path.Join(version.AgentPath, "logs/cluster-agent.log")
	//DefaultJmxLogFile points to the jmx fetch log file that will be used if not configured
	DefaultJmxLogFile = path.Join(version.AgentPath, "logs/jmxfetch.log")
	// DefaultCheckFlareDirectory a flare friendly location for checks to be written
	DefaultCheckFlareDirectory = path.Join(version.AgentPath, "logs/checks/")
	// DefaultJMXFlareDirectory a flare friendly location for jmx command logs to be written
	DefaultJMXFlareDirectory = path.Join(version.AgentPath, "logs/jmxinfo/")
	//DefaultDogstatsDLogFile points to the dogstatsd stats log file that will be used if not configured
	DefaultDogstatsDLogFile = path.Join(version.AgentPath, "logs/dogstatsd_info/dogstatsd-stats.log")
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

// GetViewsPath returns the fully qualified path to the 'gui/views' directory
func GetViewsPath() string {
	return filepath.Join(distPath, "views")
}
