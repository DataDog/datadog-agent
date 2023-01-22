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

func checkReadAccess() error {
	path := basePath
	if coreConfig.Datadog.GetBool("logs_config.use_podman_logs") {
		path = podmanBasePath
	}
	err := unix.Access(path, unix.X_OK)
	if err != nil {
		return fmt.Errorf("Error accessing %s: %w", path, err)
	}
	return nil
}

// getPath returns the file path of the container log to tail.
func getPath(id string) string {
	if coreConfig.Datadog.GetBool("logs_config.use_podman_logs") {
		return filepath.Join(podmanBasePath, fmt.Sprintf("%s/userdata/ctr.log", id))
	}
	return filepath.Join(basePath, id, fmt.Sprintf("%s-json.log", id))
}
