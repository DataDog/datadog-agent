// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package dockerpermissions

import (
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

const (
	defaultLinuxDockerSocket       = "/var/run/docker.sock"
	defaultWindowsDockerSocketPath = "//./pipe/docker_engine"
	defaultHostMountPrefix         = "/host"

	socketTimeout = 500 * time.Millisecond
)

// Check reports an issue for every Docker socket/named pipe that exists but
// whose root cause of being unreachable is a permission-denied error, via
// checkSocketPermission. Any other dial failure (busy socket, no listener,
// connection refused) is not a permission problem and is not reported.
func Check() ([]runnerdef.IssueReport, error) {
	// Check if DOCKER_HOST is set - if so, skip the check as user has custom config
	if _, dockerHostSet := os.LookupEnv("DOCKER_HOST"); dockerHostSet {
		return nil, nil
	}

	var unreachableSockets []string
	for _, socketPath := range getDockerSocketPaths() {
		exists, permissionDenied := checkSocketPermission(socketPath, socketTimeout)
		if exists && permissionDenied {
			unreachableSockets = append(unreachableSockets, socketPath)
		}
	}

	if len(unreachableSockets) > 0 {
		return []runnerdef.IssueReport{
			{
				IssueID:   IssueID,
				IssueName: IssueName,
				Source:    "docker",
				Context: map[string]string{
					"socketPaths": strings.Join(unreachableSockets, ","),
					"os":          runtime.GOOS,
				},
				Tags: []string{"docker-socket", "permissions"},
			},
		}, nil
	}

	// No issue detected
	return nil, nil
}

// getDockerSocketPaths returns the default Docker socket paths to check
func getDockerSocketPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{defaultWindowsDockerSocketPath}
	}

	// On Linux, check both with and without host mount prefix
	paths := []string{defaultLinuxDockerSocket}
	if isContainerized() {
		paths = append(paths, path.Join(defaultHostMountPrefix, defaultLinuxDockerSocket))
	}
	return paths
}

// isContainerized checks if the agent is running in a container
func isContainerized() bool {
	// Check common container indicators
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return false
}
