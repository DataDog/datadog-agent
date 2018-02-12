// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build !docker

package container

import (
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// IsAvailable returns true if there's at least one container provider, false otherwise
func IsAvailable() bool {
	return false
}

// GetContainers is the unique method that returns all containers on the host (or in the task)
// TODO: create a container interface that docker and ecs can implement
// and that other agents can consume so that we don't have to
// convert all containers to the format.
// TODO: move to a catalog and registration pattern
func GetContainers() ([]*docker.Container, error) {

	return nil, nil
}
