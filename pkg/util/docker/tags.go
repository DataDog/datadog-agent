// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
)

type SystemInfoClient interface {
	Info(ctx context.Context) (types.Info, error)
}

// GetTags returns tags that are automatically added to metrics and events on a
// host that is running docker.
func GetTags(ctx context.Context) ([]string, error) {
	du, err := GetDockerUtil()
	if err != nil {
		return nil, err
	}
	return getTags(du.cli, ctx)
}

func getTags(client SystemInfoClient, ctx context.Context) ([]string, error) {
	tags := []string{}
	info, err := client.Info(ctx)
	if err != nil {
		return tags, err
	}
	switch info.Swarm.LocalNodeState {
	case swarm.LocalNodeStateActive:
		nodeRole := swarm.NodeRoleWorker
		if info.Swarm.ControlAvailable {
			nodeRole = swarm.NodeRoleManager
		}
		tags = append(tags, fmt.Sprintf("docker_swarm_node_role:%s", nodeRole))
	default:
		break
	}
	return tags, nil
}
