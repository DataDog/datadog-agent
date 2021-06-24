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
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
)

// ListSwarmServices gets a list of all swarm services on the current node using the Docker APIs.
func (d *DockerUtil) ListSwarmServices() ([]*containers.SwarmService, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.queryTimeout)
	defer cancel()

	sList, err := dockerSwarmServices(ctx, d.cli)
	if err != nil {
		return nil, fmt.Errorf("could not get docker swarm services: %s", err)
	}

	return sList, err
}

// dockerSwarmServices returns all the swarm services in the swarm cluster
func dockerSwarmServices(ctx context.Context, client SwarmServiceAPIClient) ([]*containers.SwarmService, error) {
	services, err := client.ServiceList(ctx, types.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing swarm services: %s", err)
	}

	activeNodes, err := getActiveNodes(ctx, client)
	if err != nil {
		log.Errorf("Error getting active nodes: %s", err)
		return nil, err
	}

	ret := make([]*containers.SwarmService, 0, len(services))
	for _, s := range services {
		tasksComponents := make([]*containers.SwarmTask, 0)

		// add the serviceId filter for Tasks
		taskFilter := filters.NewArgs()
		taskFilter.Add("service", s.ID)
		// list the tasks for that service
		tasks, err := client.TaskList(ctx, types.TaskListOptions{Filters: taskFilter})
		if err != nil {
			log.Errorf("Error listing swarm tasks for Service %s: %s. Continue with the remaining services...",
				s.ID, err)
			continue
		}

		desired := uint64(0)
		running := uint64(0)

		// Replicated services have `Spec.Mode.Replicated.Replicas`, which should give this value.
		if s.Spec.Mode.Replicated != nil {
			desired = *s.Spec.Mode.Replicated.Replicas
		}
		for _, task := range tasks {

			// this should only be needed for "global" services. In future version (1.41 or up)
			// this can be directly accessed through ServiceStatus.DesiredTasks
			if s.Spec.Mode.Global != nil {
				if task.DesiredState != swarm.TaskStateShutdown {
					log.Debugf("Task having service ID %s got desired tasks for global mode", task.ServiceID)
					desired++
				}
			}
			if _, nodeActive := activeNodes[task.NodeID]; nodeActive && task.Status.State == swarm.TaskStateRunning {
				log.Debugf("Task having service ID %s is running", task.ServiceID)
				running++
			}
			taskComponent := &containers.SwarmTask{
				ID:              task.ID,
				Name:            task.Name,
				ContainerImage:  task.Spec.ContainerSpec.Image,
				ContainerSpec:   task.Spec.ContainerSpec,
				ContainerStatus: task.Status.ContainerStatus,
				DesiredState:    task.Status.State,
			}
			log.Debugf("Creating a task %s for service %s", task.Name, s.Spec.Name)
			tasksComponents = append(tasksComponents, taskComponent)
		}

		log.Debugf("Service %s has %d desired and %d running tasks", s.Spec.Name, desired, running)

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
			TaskContainers: tasksComponents,
			DesiredTasks:   desired,
			RunningTasks:   running,
		}

		ret = append(ret, service)
	}

	return ret, nil
}

func getActiveNodes(ctx context.Context, client SwarmServiceAPIClient) (map[string]bool, error) {
	nodes, err := client.NodeList(ctx, types.NodeListOptions{})
	if err != nil {
		return nil, err
	}
	activeNodes := make(map[string]bool)
	for _, n := range nodes {
		if n.Status.State == swarm.NodeStateReady {
			activeNodes[n.ID] = true
		}
	}
	return activeNodes, nil
}
