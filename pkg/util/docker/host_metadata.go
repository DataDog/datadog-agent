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

	"github.com/docker/docker/api/types/swarm"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host/container"
)

func init() {
	container.RegisterMetadataProvider("docker", getMetadata)
}

func getMetadata() (map[string]string, error) {
	if !config.IsFeaturePresent(config.Docker) {
		return nil, errors.New("Docker feature deactivated")
	}

	metadata := make(map[string]string)
	du, err := GetDockerUtil()
	if err != nil {
		return metadata, err
	}
	// short timeout to minimize metadata collection time
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	i, err := du.cli.Info(ctx)
	if err != nil {
		return metadata, err
	}
	metadata["docker_version"] = i.ServerVersion
	metadata["docker_swarm"] = "inactive"
	if i.Swarm.LocalNodeState == swarm.LocalNodeStateActive {
		metadata["docker_swarm"] = "active"
	}

	return metadata, nil
}
