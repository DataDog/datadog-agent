// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package dockerpermissions

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

const (
	defaultLinuxDockerSocket       = "/var/run/docker.sock"
	defaultWindowsDockerSocketPath = "//./pipe/docker_engine"
	defaultHostMountPrefix         = "/host"

	socketTimeout = 500 * time.Millisecond
)

// check reports an issue for every Docker socket/named pipe that exists but is unreachable because of a permission error.
func check(hostname hostnameinterface.Component) ([]runnerdef.IssueReport, error) {
	// Check if DOCKER_HOST is set - if so, skip the check as user has custom config
	if _, dockerHostSet := os.LookupEnv("DOCKER_HOST"); dockerHostSet {
		return nil, nil
	}

	var unreachableSockets []string
	for _, socketPath := range getDockerSocketPaths() {
		exists, err := socket.IsAvailable(socketPath, socketTimeout)
		if exists && errors.Is(err, os.ErrPermission) {
			unreachableSockets = append(unreachableSockets, socketPath)
		}
	}

	if len(unreachableSockets) > 0 {
		// Sort so the socketPaths string and the id digest are order-independent.
		sort.Strings(unreachableSockets)
		return []runnerdef.IssueReport{
			{
				IssueID:   socketSetIssueID(hostname.GetSafe(context.Background()), unreachableSockets),
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

// socketSetIssueID scopes IssueID by hostname and the unreachable socket set so each host's socket problems file a distinct issue; caller passes a sorted slice.
func socketSetIssueID(hostname string, sortedSockets []string) string {
	h := fnv.New64a()
	h.Write([]byte(hostname)) // never returns an error for hash.Hash
	h.Write([]byte{0})        // delimiter between hostname and sockets
	for _, socketPath := range sortedSockets {
		h.Write([]byte(socketPath))
		h.Write([]byte{0}) // delimiter so {"a","bc"} and {"ab","c"} differ
	}
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
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
