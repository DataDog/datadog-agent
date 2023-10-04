// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// getEnv retrieves the environment variable key. If it does not exist it returns the default.
func getEnv(key string, dfault string) string {
	value := os.Getenv(key)
	if value == "" {
		value = dfault
	}
	return value
}

// GetDockerSocketPath is only for exposing the sockpath out of the module
func GetDockerSocketPath() (string, error) {
	// If we don't have a docker.sock then return a known error.
	sockPath := getEnv("DOCKER_SOCKET_PATH", "/var/run/docker.sock")
	if !filesystem.FileExists(sockPath) {
		return "", docker.ErrDockerNotAvailable
	}
	return sockPath, nil
}
