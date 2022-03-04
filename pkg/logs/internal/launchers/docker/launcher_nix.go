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

	"github.com/DataDog/datadog-agent/pkg/config"

	"golang.org/x/sys/unix"
)

const (
	basePath       = "/var/lib/docker/containers"
	podmanBasePath = "/var/lib/containers/storage/overlay-containers"
)

func (l *Launcher) checkContainerLogfileAccess() error {
	var path string

	// determine the base path that getContainerLogfilePath will use
	switch l.runtime {
	case config.Podman:
		path = podmanBasePath
	default: // ..default to config.Docker
		path = basePath
	}

	err := unix.Access(path, unix.X_OK)
	if err != nil {
		return fmt.Errorf("Error accessing %s: %w", path, err)
	}
	return nil
}

// getContainerLogfilePath returns the file path of the container log to tail.
func (l *Launcher) getContainerLogfilePath(id string) string {
	switch l.runtime {
	case config.Podman:
		return filepath.Join(podmanBasePath, fmt.Sprintf("%s/userdata/ctr.log", id))
	default: // ..default to config.Docker
		return filepath.Join(basePath, id, fmt.Sprintf("%s-json.log", id))
	}
}
