// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build netbsd || openbsd || solaris || dragonfly || linux

package path

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
)

const (
	// DefaultConfPath points to the folder containing datadog.yaml
	DefaultConfPath = "/etc/datadog-agent"
	// DefaultLogFile points to the log file that will be used if not configured
	DefaultLogFile = "/var/log/datadog/agent.log"
	// DefaultDCALogFile points to the log file that will be used if not configured
	DefaultDCALogFile = "/var/log/datadog/cluster-agent.log"
	// DefaultJmxLogFile points to the jmx fetch log file that will be used if not configured
	DefaultJmxLogFile = "/var/log/datadog/jmxfetch.log"
	// DefaultCheckFlareDirectory a flare friendly location for checks to be written
	DefaultCheckFlareDirectory = "/var/log/datadog/checks/"
	// DefaultJMXFlareDirectory a flare friendly location for jmx command logs to be written
	DefaultJMXFlareDirectory = "/var/log/datadog/jmxinfo/"
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

// GetViewsPath returns the fully qualified path to the 'gui/views' directory
func GetViewsPath() string {
	return filepath.Join(distPath, "views")
}
