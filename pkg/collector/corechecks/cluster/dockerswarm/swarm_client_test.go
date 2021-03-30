// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/util/containers"
	"github.com/docker/docker/api/types/swarm"
	"time"
)

var swarmService = containers.SwarmService{
	ID:             "klbo61rrhksdmc9ho3pq97t6e",
	Name:           "agent_stackstate-agent",
	ContainerImage: "stackstate/stackstate-agent-2-test:stac-12057-swarm-topology@sha256:1d463af3e8c407e08bff9f6127e4959d5286a25018ec5269bfad5324815eb367",
	Labels: map[string]string{
		"com.docker.stack.image":     "docker.io/stackstate/stackstate-agent-2-test:stac-12057-swarm-topology",
		"com.docker.stack.namespace": "agent",
	},
	Version:   swarm.Version{Index: 136},
	CreatedAt: time.Date(2021, time.March, 10, 23, 0, 0, 0, time.UTC),
	UpdatedAt: time.Date(2021, time.March, 10, 45, 0, 0, 0, time.UTC),
	TaskContainers: []*containers.SwarmTask{
		{
			ID:             "qwerty12345",
			Name:           "/agent_stackstate-agent.1.skz8sp5d1y4f64qykw37mf3k2",
			ContainerImage: "stackstate/stackstate-agent-2-test",
			ContainerStatus: swarm.ContainerStatus{
				ContainerID: "a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
				ExitCode:    0,
				PID:         341,
			},
			DesiredState: swarm.TaskStateRunning,
		},
	},
	DesiredTasks: 2,
	RunningTasks: 2,
}

// MockSwarmClient - used in testing
type MockSwarmClient struct {
}

// ListSwarmServices returns a mock list of services
func (m *MockSwarmClient) ListSwarmServices() ([]*containers.SwarmService, error) {
	swarmServices := []*containers.SwarmService{
		&swarmService,
	}
	return swarmServices, nil
}

// MockSwarmFactory is exported for unit testing with MockSwarmClient to produce mock outputs
func MockSwarmFactory() check.Check {
	return &SwarmCheck{
		CheckBase:         core.NewCheckBase(SwarmCheckName),
		instance:          &SwarmConfig{},
		topologyCollector: makeSwarmTopologyCollector(&MockSwarmClient{}),
	}
}
