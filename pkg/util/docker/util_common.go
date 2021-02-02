// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

var (
	// ErrNotImplemented is the "not implemented" error given by `gopsutil` when an
	// OS doesn't support and API. Unfortunately it's in an internal package so
	// we can't import it so we'll copy it here.
	ErrNotImplemented = errors.New("not implemented yet")

	// ErrDockerNotAvailable is returned if Docker is not running on the current machine.
	// We'll use this when configuring the DockerUtil so we don't error on non-docker machines.
	ErrDockerNotAvailable = errors.New("docker not available")

	// ErrDockerNotCompiled is returned if docker support is not compiled in.
	// User classes should handle that case as gracefully as possible.
	ErrDockerNotCompiled = errors.New("docker support not compiled in")
)

// ContainerIDToEntityName returns a prefixed entity name from a container ID
func ContainerIDToEntityName(cid string) string {
	return containers.BuildEntityName(containers.RuntimeNameDocker, cid)
}

// ContainerIDToTaggerEntityName returns a prefixed entity name from a container ID
func ContainerIDToTaggerEntityName(cid string) string {
	return containers.BuildTaggerEntityName(cid)
}
