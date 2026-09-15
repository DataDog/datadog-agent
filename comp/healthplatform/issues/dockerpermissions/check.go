// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package dockerpermissions

import (
	"errors"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

const (
	defaultLinuxDockerSocket       = "/var/run/docker.sock"
	defaultWindowsDockerSocketPath = "//./pipe/docker_engine"
	defaultHostMountPrefix         = "/host"

	socketTimeout = 500 * time.Millisecond
)

// Check reports an issue for every Docker socket/named pipe that exists but
// is unreachable: a permission-denied error is reported as
// "Docker Socket Permission", any other dial failure (connection refused,
// stale socket, daemon down) is reported as "Docker Socket Unavailable".
func (c *checker) Check() ([]runnerdef.IssueReport, error) {
	// Check if DOCKER_HOST is set - if so, skip the check as user has custom config
	if _, dockerHostSet := os.LookupEnv("DOCKER_HOST"); dockerHostSet {
		return nil, nil
	}

	permissionSockets, unavailableSockets := classifySockets(getDockerSocketPaths())

	var reports []runnerdef.IssueReport
	if len(permissionSockets) > 0 {
		reports = append(reports, runnerdef.IssueReport{
			IssueID:   c.instanceIssueID(IssueID),
			IssueName: IssueName,
			Source:    "docker",
			Context: map[string]string{
				"socketPaths": strings.Join(permissionSockets, ","),
				"os":          runtime.GOOS,
			},
			Tags: []string{"docker-socket", "permissions"},
		})
	}
	if len(unavailableSockets) > 0 {
		reports = append(reports, runnerdef.IssueReport{
			IssueID:   c.instanceIssueID(SocketUnavailableIssueID),
			IssueName: SocketUnavailableIssueName,
			Source:    "docker",
			Context: map[string]string{
				"socketPaths": strings.Join(unavailableSockets, ","),
				"os":          runtime.GOOS,
			},
			Tags: []string{"docker-socket", "unavailable"},
		})
	}

	return reports, nil
}

// classifySockets partitions socketPaths by why they are unreachable: a
// permission-denied error, or any other dial failure (connection refused,
// stale socket, daemon down). Paths that don't exist or are reachable are
// omitted from both slices.
func classifySockets(socketPaths []string) (permissionSockets, unavailableSockets []string) {
	for _, socketPath := range socketPaths {
		exists, err := socket.IsAvailable(socketPath, socketTimeout)
		switch {
		case !exists || err == nil:
			// absent or reachable -> not an issue
		case errors.Is(err, os.ErrPermission):
			permissionSockets = append(permissionSockets, socketPath)
		default:
			unavailableSockets = append(unavailableSockets, socketPath)
		}
	}
	return permissionSockets, unavailableSockets
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
