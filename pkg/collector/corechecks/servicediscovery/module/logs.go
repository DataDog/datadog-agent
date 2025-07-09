// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// isLogFile is used from the getOpenFilesInfo function to filter paths which
// look like log files.
func isLogFile(path string) bool {
	if !strings.HasSuffix(path, ".log") {
		return false
	}

	// Skip log files from Docker containers and Kubernetes pods since these
	// are collected by other means.
	if strings.HasPrefix(path, "/var/lib/docker/containers") ||
		strings.HasPrefix(path, "/var/log/pods") {
		return false
	}

	return true
}

// getLogFiles takes a list of candidate file paths which look like log files
// and ensures that they are opened with O_WRONLY, in order to avoid false
// positives.
func getLogFiles(pid int32, candidates []fdPath) []string {
	seen := make(map[string]struct{}, len(candidates))
	logs := make([]string, 0, len(candidates))

	for _, candidate := range candidates {
		if _, ok := seen[candidate.path]; ok {
			continue
		}

		fdInfoPath := kernel.HostProc(fmt.Sprintf("%d/fdinfo/%s", pid, candidate.fd))
		fdInfo, err := os.ReadFile(fdInfoPath)
		if err != nil {
			continue
		}

		lines := strings.SplitN(string(fdInfo), "\n", 2)
		if len(lines) < 2 || !strings.HasPrefix(lines[1], "flags:") {
			continue
		}
		fields := strings.Fields(lines[1])
		if len(fields) < 2 {
			continue
		}
		flags, err := strconv.ParseUint(fields[1], 8, 32)
		if err != nil {
			continue
		}
		// O_WRONLY is 1
		if flags&1 == 1 {
			logs = append(logs, candidate.path)
			seen[candidate.path] = struct{}{}
		}
	}
	return logs
}
