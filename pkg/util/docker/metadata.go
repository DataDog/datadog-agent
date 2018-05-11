// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types/swarm"

	"github.com/DataDog/datadog-agent/pkg/metadata/host/container"
)

func init() {
	container.RegisterMetadataProvider("docker", getMetadata)
}

func getMetadata() map[string]string {
	metadata := make(map[string]string)
	du, err := GetDockerUtil()
	if err != nil {
		log.Debugf("Unable to collect Docker host metadata: %s", err)
		return metadata
	}
	// short timeout to minimize metadata collection time
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	i, err := du.cli.Info(ctx)
	if err != nil {
		return metadata
	}
	metadata["docker_version"] = i.ServerVersion
	metadata["docker_swarm"] = "inactive"
	if i.Swarm.LocalNodeState == swarm.LocalNodeStateActive {
		metadata["docker_swarm"] = "active"
	}

	return metadata
}
