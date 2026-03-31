// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package dockerlogsrotation

import (
	"os"
	"path"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

const (
	defaultLinuxDockerSocket = "/var/run/docker.sock"
	defaultLinuxDockerLogDir = "/var/lib/docker/containers"
	defaultHostMountPrefix   = "/host"
)

// Check detects if Docker log collection is configured in a way that risks losing logs
// after Docker log rotation events. Returns an issue if:
//   - Logs are enabled AND container_collect_all is true
//   - Docker socket is accessible (so dockerpermissions check is not relevant)
//   - docker_container_use_file is NOT enabled (socket-based tailing is fragile under rotation)
func Check() (*healthplatform.IssueReport, error) {
	// Skip if DOCKER_HOST is set — user has custom config, let them manage it
	if _, dockerHostSet := os.LookupEnv("DOCKER_HOST"); dockerHostSet {
		return nil, nil
	}

	// Check if Docker log collection is enabled via environment
	logsEnabled := os.Getenv("DD_LOGS_ENABLED") == "true"
	containerCollectAll := os.Getenv("DD_CONTAINER_COLLECT_ALL") == "true" ||
		os.Getenv("DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL") == "true"

	if !logsEnabled || !containerCollectAll {
		// Not using Docker log collection, nothing to check
		return nil, nil
	}

	// Check Docker socket presence — if absent, dockerpermissions check fires instead
	dockerSocketPath := defaultLinuxDockerSocket
	if isContainerized() {
		hostSocketPath := path.Join(defaultHostMountPrefix, defaultLinuxDockerSocket)
		if _, err := os.Stat(hostSocketPath); err == nil {
			dockerSocketPath = hostSocketPath
		}
	}
	if _, err := os.Stat(dockerSocketPath); os.IsNotExist(err) {
		return nil, nil // Docker not present; different issue
	}

	// If file-based tailing is enabled, rotation is handled correctly — no issue
	dockerUseFile := os.Getenv("DD_LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE") == "true"
	if dockerUseFile {
		return nil, nil
	}

	// Determine the Docker log directory to report
	dockerLogDir := defaultLinuxDockerLogDir
	if isContainerized() {
		hostLogDir := path.Join(defaultHostMountPrefix, defaultLinuxDockerLogDir)
		if _, err := os.Stat(hostLogDir); err == nil {
			dockerLogDir = hostLogDir
		}
	}

	// Socket-based tailing is in use — flag the rotation risk
	return &healthplatform.IssueReport{
		IssueId: IssueID,
		Context: map[string]string{
			"dockerLogDir":   dockerLogDir,
			"reason":         "socket-tailing-rotation-risk",
			"recommendation": "enable-file-tailing",
		},
		Tags: []string{"docker", "logs", "rotation", "socket-tailing"},
	}, nil
}

// isContainerized checks if the agent is running in a container
func isContainerized() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return false
}
