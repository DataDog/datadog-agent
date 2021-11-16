// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker,!windows

package docker

import (
	"fmt"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"golang.org/x/sys/unix"
)

const (
	basePath = "/var/lib/docker/containers"
)

func checkReadAccess() error {
	path := basePath
	if coreConfig.Datadog.GetBool("logs_config.use_podman_logs") {
		path = "/var/lib/containers/storage/overlay-containers"
	}
	err := unix.Access(path, unix.X_OK)
	if err != nil {
		return fmt.Errorf("Error accessing %s: %w", path, err)
	}
	return nil
}
