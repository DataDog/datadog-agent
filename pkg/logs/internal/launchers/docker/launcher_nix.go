// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && !windows
// +build docker,!windows

package docker

import (
	"fmt"
	"path/filepath"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"

	"golang.org/x/sys/unix"
)

const (
	basePath       = "/var/lib/docker/containers"
	podmanBasePath = "/var/lib/containers/storage/overlay-containers"
)

// TODO: Perhaps think of a better name for this function?
func getDockerLogsPath() string {
	usePodmanLogs := coreConfig.Datadog.GetBool("logs_config.use_podman_logs")
	overridePath := coreConfig.Datadog.GetString("logs_config.docker_path_override")

	if usePodmanLogs {
		return podmanBasePath
	} else if len(overridePath) > 0 {
		return overridePath
	}

	return basePath
}

func checkReadAccess() error {
	path := getDockerLogsPath()

	err := unix.Access(path, unix.X_OK)
	if err != nil {
		return fmt.Errorf("Error accessing %s: %w", path, err)
	}
	return nil
}

// getPath returns the file path of the container log to tail.
func getPath(id string) string {
	path := getDockerLogsPath()

	if coreConfig.Datadog.GetBool("logs_config.use_podman_logs") {
		return filepath.Join(path, fmt.Sprintf("%s/userdata/ctr.log", id))
	} else if len(coreConfig.Datadog.GetString("logs_config.docker_path_override")) > 0 {
		return filepath.Join(path, id, fmt.Sprintf("%s-json.log", id))

	}
	return filepath.Join(path, id, fmt.Sprintf("%s-json.log", id))
}
