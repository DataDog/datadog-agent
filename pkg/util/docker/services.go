// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/util/containers"
	"github.com/docker/docker/api/types"
)

// ListSwarmServices gets a list of all swarm services on the current node using the Docker APIs.
func (d *DockerUtil) ListSwarmServices() ([]*containers.SwarmService, error) {
	sList, err := d.dockerSwarmServices()
	if err != nil {
		return nil, fmt.Errorf("could not get docker swarm services: %s", err)
	}

	return sList, err
}

// dockerSwarmServices returns all the swarm services in the swarm cluster
func (d *DockerUtil) dockerSwarmServices() ([]*containers.SwarmService, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()
	sList, err := d.cli.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing swarm services: %s", err)
	}
	ret := make([]*containers.SwarmService, 0, len(sList))
	for _, s := range sList {
		service := &containers.SwarmService{
			ID:             s.ID,
			Name:           s.Spec.Name,
			ContainerImage: s.Spec.TaskTemplate.ContainerSpec.Image,
			Labels:         s.Spec.Labels,
			Version:        s.Version,
			CreatedAt:      s.CreatedAt,
			UpdatedAt:      s.UpdatedAt,
			Spec:           s.Spec,
			PreviousSpec:   s.PreviousSpec,
			Endpoint:       s.Endpoint,
			UpdateStatus:   s.UpdateStatus,
		}

		ret = append(ret, service)
	}

	return ret, nil
}
