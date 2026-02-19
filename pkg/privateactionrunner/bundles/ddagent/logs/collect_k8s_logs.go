// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"os"
	"path/filepath"
	"strings"
)

// k8sLogDirs are the host directories that contain Kubernetes log files.
var k8sLogDirs = []string{
	"/var/log/pods",
	"/var/log/containers",
}

// collectK8sLogs walks Kubernetes log directories and returns all *.log files
// found. Non-existent directories are silently skipped.
func collectK8sLogs(hostPrefix string) ([]LogFileEntry, []string) {
	var entries []LogFileEntry
	var errs []string

	for _, dir := range k8sLogDirs {
		hostDir := toHostPath(hostPrefix, dir)

		if _, err := os.Stat(hostDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.WalkDir(hostDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // skip entries we cannot read
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(d.Name(), ".log") {
				entries = append(entries, LogFileEntry{
					Path:   toOutputPath(hostPrefix, path),
					Source: "kubernetes",
				})
			}
			return nil
		})
		if err != nil {
			errs = append(errs, "error walking "+dir+": "+err.Error())
		}
	}
	return entries, errs
}
