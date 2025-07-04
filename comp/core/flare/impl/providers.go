// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flareimpl

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// Match .yaml and .yml to ship configuration files in the flare.
var cnfFileExtRx = regexp.MustCompile(`(?i)\.ya?ml`)

func getFirstSuffix(s string) string {
	return filepath.Ext(strings.TrimSuffix(s, filepath.Ext(s)))
}

func (f *flareComp) collectLogsFiles(fb types.FlareBuilder) error {
	logFile := f.config.GetString("log_file")
	if logFile == "" {
		logFile = f.params.DefaultLogFile
	}

	jmxLogFile := f.config.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = f.params.DefaultJMXLogFile
	}

	dogstatsdLogFile := f.config.GetString("dogstatsd_log_file")
	if dogstatsdLogFile == "" {
		dogstatsdLogFile = f.params.DefaultDogstatsdLogFile
	}

	shouldIncludeFunc := func(path string) bool {
		if filepath.Ext(path) == ".log" || getFirstSuffix(path) == ".log" {
			return true
		}
		return false
	}

	f.log.Flush()
	fb.CopyDirToWithoutScrubbing(filepath.Dir(logFile), "logs", shouldIncludeFunc)                         //nolint:errcheck
	fb.CopyDirToWithoutScrubbing(filepath.Dir(jmxLogFile), "logs", shouldIncludeFunc)                      //nolint:errcheck
	fb.CopyDirToWithoutScrubbing(filepath.Dir(dogstatsdLogFile), "logs/dogstatsd_info", shouldIncludeFunc) //nolint:errcheck
	return nil
}

func (f *flareComp) collectConfigFiles(fb types.FlareBuilder) error {
	confSearchPaths := map[string]string{
		"":        f.config.GetString("confd_path"),
		"fleet":   filepath.Join(f.config.GetString("fleet_policies_dir"), "conf.d"),
		"dist":    filepath.Join(f.params.DistPath, "conf.d"),
		"checksd": f.params.PythonChecksPath,
	}

	for prefix, filePath := range confSearchPaths {
		fb.CopyDirTo(filePath, filepath.Join("etc", "confd", prefix), func(path string) bool { //nolint:errcheck
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

	return nil
}
