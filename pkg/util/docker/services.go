// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"errors"
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/util/containers"
	"github.com/docker/docker/api/types"
	"time"
)

// ServiceListConfig allows to pass listing options
type ServiceListConfig struct {
}

// Containers gets a list of all containers on the current node using a mix of
// the Docker APIs and cgroups stats. We attempt to limit syscalls where possible.
func (d *DockerUtil) ListSwarmServices(cfg *ContainerListConfig) ([]*containers.Container, error) {
	if cfg == nil {
		return nil, errors.New("configuration is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	cList, err := d.cli.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing docker swarm services: %s", err)
	}
	ret := make([]*containers.Container, 0, len(cList))
	for _, c := range cList {

		container := &containers.Container{
			Type:     "Docker",
			ID:       c.ID,
			EntityID: entityID,
			Name:     c.Names[0],
			Image:    image,
			ImageID:  c.ImageID,
			Created:  c.Created,
			State:    c.State,
			Excluded: excluded,
			Health:   parseContainerHealth(c.Status),
			Mounts: c.Mounts,
		}

		ret = append(ret, container)
	}

	return ret, nil
}
