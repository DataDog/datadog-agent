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
	"github.com/moby/moby/client"
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
	return getTags(ctx, du.cli)
}

func getTags(ctx context.Context, c client.SystemAPIClient) ([]string, error) {
	tags := []string{}
	result, err := c.Info(ctx, client.InfoOptions{})
	if err != nil {
		return tags, err
	}
	switch result.Info.Swarm.LocalNodeState {
	case swarm.LocalNodeStateActive:
		nodeRole := swarm.NodeRoleWorker
		if result.Info.Swarm.ControlAvailable {
			nodeRole = swarm.NodeRoleManager
		}
		tags = append(tags, fmt.Sprintf("docker_swarm_node_role:%s", nodeRole))
	default:
		break
	}
	return tags, nil
}
