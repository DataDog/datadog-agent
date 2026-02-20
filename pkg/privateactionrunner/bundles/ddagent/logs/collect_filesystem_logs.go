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

// collectFilesystemLogs walks /var/log (excluding K8s/Docker subdirs) plus
// any user-specified additional directories.
func collectFilesystemLogs(hostPrefix string, additionalDirs []string) ([]FileEntry, []string) {
	var entries []FileEntry
	var errs []string

	// Walk /var/log with the isLogFile heuristic
	varLogDir := toHostPath(hostPrefix, "/var/log")
	if _, err := os.Stat(varLogDir); err == nil {
		err := filepath.WalkDir(varLogDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			outputPath := toOutputPath(hostPrefix, path)
			if isLogFile(outputPath) {
				entries = append(entries, FileEntry{
					Path:   outputPath,
					Source: "filesystem",
				})
			}
			return nil
		})
		if err != nil {
			errs = append(errs, "error walking /var/log: "+err.Error())
		}
	}

	// Walk additional directories — include all regular files
	for _, dir := range additionalDirs {
		hostDir := toHostPath(hostPrefix, dir)
		if _, err := os.Stat(hostDir); os.IsNotExist(err) {
			errs = append(errs, "additional directory not found: "+dir)
			continue
		}
		err := filepath.WalkDir(hostDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			entries = append(entries, FileEntry{
				Path:   toOutputPath(hostPrefix, path),
				Source: "filesystem",
			})
			return nil
		})
		if err != nil {
			errs = append(errs, "error walking "+dir+": "+err.Error())
		}
	}

	return entries, errs
}

// isLogFile mirrors the heuristic from pkg/discovery/module/logs.go but
// without the //go:build linux constraint so it can run cross-platform.
func isLogFile(path string) bool {
	// Files in /var/log are log files even if they don't end with .log.
	if strings.HasPrefix(path, "/var/log/") {
		// Ignore Kubernetes pods logs since they are collected by other means.
		if strings.HasPrefix(path, "/var/log/pods") {
			return false
		}
		// Ignore Kubernetes container symlinks.
		if strings.HasPrefix(path, "/var/log/containers") {
			return false
		}
		// Ignore Docker container logs.
		if strings.HasPrefix(path, "/var/log/docker") {
			return false
		}
		return true
	}

	if strings.HasSuffix(path, ".log") {
		// Ignore Docker container logs since they are collected by other means.
		return !strings.HasPrefix(path, "/var/lib/docker/containers")
	}

	return false
}
