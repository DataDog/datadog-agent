// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"path/filepath"
	"regexp"
	"strings"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

var (
	// Match .yaml and .yml to ship configuration files in the flare.
	cnfFileExtRx = regexp.MustCompile(`(?i)\.ya?ml`)
)

func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
}

func (f *flare) collectLogsFiles(fb flarehelpers.FlareBuilder) error {
	logFile := f.config.GetString("log_file")
	if logFile == "" {
		logFile = f.params.defaultLogFile
	}

	jmxLogFile := f.config.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = f.params.defaultJMXLogFile
	}

	shouldIncludeFunc := func(path string) bool {
		if filepath.Ext(path) == ".log" || getFirstSuffix(path) == ".log" {
			return true
		}
		return false
	}

	f.log.Flush()
	fb.CopyDirToWithoutScrubbing(filepath.Dir(logFile), "logs", shouldIncludeFunc)
	fb.CopyDirToWithoutScrubbing(filepath.Dir(jmxLogFile), "logs", shouldIncludeFunc)
	return nil
}

func (f *flare) collectConfigFiles(fb flarehelpers.FlareBuilder) error {
	confSearchPaths := map[string]string{
		"":        f.config.GetString("confd_path"),
		"dist":    filepath.Join(f.params.distPath, "conf.d"),
		"checksd": f.params.pythonChecksPath,
	}

	for prefix, filePath := range confSearchPaths {
		fb.CopyDirTo(filePath, filepath.Join("etc", "confd", prefix), func(path string) bool {
			// ignore .example file
			if filepath.Ext(path) == ".example" {
				return false
			}

			firstSuffix := []byte(getFirstSuffix(path))
			ext := []byte(filepath.Ext(path))
			if cnfFileExtRx.Match(firstSuffix) || cnfFileExtRx.Match(ext) {
				return true
			}
			return false
		})
	}

	if mainConfpath := f.config.ConfigFileUsed(); mainConfpath != "" {
		confDir := filepath.Dir(mainConfpath)

		// zip up the config file that was actually used, if one exists
		fb.CopyFileTo(mainConfpath, filepath.Join("etc", "datadog.yaml"))

		// figure out system-probe file path based on main config path, and use best effort to include
		// system-probe.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "system-probe.yaml"), filepath.Join("etc", "system-probe.yaml"))

		// use best effort to include security-agent.yaml to the flare
		fb.CopyFileTo(filepath.Join(confDir, "security-agent.yaml"), filepath.Join("etc", "system-probe.yaml"))
	}
	return nil
}
