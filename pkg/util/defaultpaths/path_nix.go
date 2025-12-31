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

const (
	// ConfPath points to the folder containing datadog.yaml
	ConfPath = "/etc/datadog-agent"
	// LogFile points to the log file that will be used if not configured
	LogFile = "/var/log/datadog/agent.log"
	// DCALogFile points to the log file that will be used if not configured
	DCALogFile = "/var/log/datadog/cluster-agent.log"
	// JmxLogFile points to the jmx fetch log file that will be used if not configured
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

// CommonRootOrPath will replace the known linux filesystem hierarchy paths with a common root
//
//	/etc/datadog-agent/** -> {root}/etc/**
//	/var/log/datadog/** -> {root}/logs/**
//	/var/run/datadog/** -> {root}/run/**
func CommonRootOrPath(root, path string) string {
	if root == "" {
		return path
	}

	if strings.Contains("log", path) {
		rest := strings.TrimPrefix(path, "/var/log/datadog/")
		return filepath.Join(root, "logs", rest)
	}
	if strings.Contains("etc", path) {
		rest := strings.TrimPrefix(path, "/etc/datadog-agent/")
		return filepath.Join(root, "etc", rest)
	}
	if strings.Contains("run", path) {
		rest := strings.TrimPrefix(path, "/var/run/datadog/")
		return filepath.Join(root, "run", rest)
	}

	return path
}
