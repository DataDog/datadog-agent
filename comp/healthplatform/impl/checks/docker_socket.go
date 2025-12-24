// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !serverless

// Package checks provides health checks for the health platform component.
package checks

import (
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/remediations/dockerpermissions"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

const (
	// DockerSocketCheckID is the health check ID for the Docker socket check
	DockerSocketCheckID = "docker-socket-access"
	// DockerSocketCheckName is the human-readable name for the health check
	DockerSocketCheckName = "Docker Socket Access"
	// defaultDockerSocket is the default path to the Docker socket on Linux
	defaultDockerSocket = "/var/run/docker.sock"
	// socketTimeout is the timeout for checking socket accessibility
	socketTimeout = 500 * time.Millisecond
)

// CheckDockerSocket checks if the Docker socket is accessible.
// Returns the issue ID and context if there's an issue, or empty string if no issue.
func CheckDockerSocket() (string, map[string]string) {
	socketPath := defaultDockerSocket
	exists, reachable := socket.IsAvailable(socketPath, socketTimeout)

	// No issue if socket doesn't exist (Docker not installed) or is accessible
	if !exists || reachable {
		return "", nil
	}

	// Socket exists but is not accessible (permission denied)
	return dockerpermissions.IssueIDSocket, map[string]string{
		"type":       "socket",
		"socketPath": socketPath,
		"os":         runtime.GOOS,
	}
}
