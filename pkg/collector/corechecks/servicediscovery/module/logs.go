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
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// isLogFile is used from the getOpenFilesInfo function to filter paths which
// look like log files.
func isLogFile(path string) bool {
	// Files in /var/log are log files even if they don't end with .log.
	if strings.HasPrefix(path, "/var/log/") {
		// Ignore Kubernetes pods logs since they are collected by other means.
		return !strings.HasPrefix(path, "/var/log/pods")
	}

	if strings.HasSuffix(path, ".log") {
		// Ignore Docker container logs since they are collected by other means.
		return !strings.HasPrefix(path, "/var/lib/docker/containers")
	}

	return false
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
		const wanted = syscall.O_WRONLY | syscall.O_APPEND
		if flags&wanted == wanted {
			logs = append(logs, candidate.path)
			seen[candidate.path] = struct{}{}
		} else {
			log.Tracef("Ignoring potential log file %s from pid %d due to flags: %#x (%O)", candidate.path, pid, flags, flags)
		}
	}
	return logs
}
