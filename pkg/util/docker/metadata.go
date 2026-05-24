// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

// GetMetadata returns metadata about the docker runtime such as docker_version and if docker_swarm is enabled or not.
func GetMetadata() (map[string]string, error) {
	if !env.IsFeaturePresent(env.Docker) {
		return nil, errors.New("Docker feature deactivated")
	}

	du, err := GetDockerUtil()
	if err != nil {
		return nil, err
	}

	// short timeout to minimize metadata collection time
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := du.cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return nil, err
	}

	dockerSwarm := "inactive"
	if result.Info.Swarm.LocalNodeState == swarm.LocalNodeStateActive {
		dockerSwarm = "active"
	}

	return map[string]string{
		"docker_version": result.Info.ServerVersion,
		"docker_swarm":   dockerSwarm,
	}, nil
}
