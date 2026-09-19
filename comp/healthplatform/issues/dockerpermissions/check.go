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

	unixSocketPrefix   = "unix://"
	winNamedPipePrefix = "npipe://"

	socketTimeout = 500 * time.Millisecond
)

// Check reports an issue for every Docker socket/named pipe that is unreachable, split by permission-denied vs. other dial failures, skipping or narrowing to the DOCKER_HOST-selected socket when it's set.
func (c *checker) Check() ([]runnerdef.IssueReport, error) {
	socketPaths := getDockerSocketPaths()
	if host, ok := os.LookupEnv("DOCKER_HOST"); ok {
		matched, isDefault := matchingDefaultSocketPath(host, socketPaths)
		if !isDefault {
			return nil, nil
		}
		socketPaths = []string{matched}
	}

	permissionSockets, unavailableSockets := classifySockets(socketPaths)

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

// classifySockets partitions socketPaths into permission-denied and other-dial-failure paths, omitting ones that don't exist or are reachable.
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

// matchingDefaultSocketPath reports which entry in socketPaths host resolves to, as opposed to a genuine user-configured custom endpoint.
func matchingDefaultSocketPath(host string, socketPaths []string) (path string, ok bool) {
	prefix := unixSocketPrefix
	if runtime.GOOS == "windows" {
		prefix = winNamedPipePrefix
	}
	for _, p := range socketPaths {
		if host == prefix+p {
			return p, true
		}
	}
	return "", false
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
