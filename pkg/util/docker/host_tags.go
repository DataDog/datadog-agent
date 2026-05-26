// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/api/types/system"
)

// GetTags returns tags that are automatically added to metrics and events on a
// host that is running docker.
func GetTags(ctx context.Context) ([]string, error) {
	du, err := GetDockerUtil()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	info, err := safeInfo(ctx, du.cli)
	if err != nil {
		return []string{}, err
	}
	return buildSwarmTags(info), nil
}

// buildSwarmTags derives the docker swarm-related host tags from a daemon
// /info response.
func buildSwarmTags(info system.Info) []string {
	if info.Swarm.LocalNodeState != swarm.LocalNodeStateActive {
		return []string{}
	}
	nodeRole := swarm.NodeRoleWorker
	if info.Swarm.ControlAvailable {
		nodeRole = swarm.NodeRoleManager
	}
	return []string{fmt.Sprintf("docker_swarm_node_role:%s", nodeRole)}
}
