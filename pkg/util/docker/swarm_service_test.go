// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"github.com/StackVista/stackstate-agent/pkg/util/containers"
	"github.com/docker/docker/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

var (
	serviceLists = swarm.Service{
		ID: "klbo61rrhksdmc9ho3pq97t6e",
		Meta: swarm.Meta{
			Version:   swarm.Version{Index: 136},
			CreatedAt: time.Date(2021, time.March, 10, 23, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2021, time.March, 10, 45, 0, 0, 0, time.UTC),
		},
		Spec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{
				Name: "agent_stackstate-agent",
				Labels: map[string]string{
					"com.docker.stack.image":     "docker.io/stackstate/stackstate-agent-2-test:stac-12057-swarm-topology",
					"com.docker.stack.namespace": "agent",
				},
			},
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: swarm.ContainerSpec{
					Image: "stackstate/stackstate-agent-2-test:stac-12057-swarm-topology@sha256:1d463af3e8c407e08bff9f6127e4959d5286a25018ec5269bfad5324815eb367",
				},
			},
			Mode: swarm.ServiceMode{
				Replicated: &swarm.ReplicatedService{Replicas: createIntPointer(1)},
			},
		},
	}
	taskLists = swarm.Task{
		ID: "qwerty12345",
		Annotations: swarm.Annotations{
			Name: "/agent_stackstate-agent.1.skz8sp5d1y4f64qykw37mf3k2",
		},
		Spec: swarm.TaskSpec{
			ContainerSpec: swarm.ContainerSpec{
				Image: "stackstate/stackstate-agent-2-test:stac-12057-swarm-topology@sha256:1d463af3e8c407e08bff9f6127e4959d5286a25018ec5269bfad5324815eb367",
			},
		},
		ServiceID: "klbo61rrhksdmc9ho3pq97t6e",
		NodeID:    "NodeStateReady",
		Status: swarm.TaskStatus{
			State: "running",
			ContainerStatus: swarm.ContainerStatus{
				ContainerID: "a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
				PID:         341,
				ExitCode:    0,
			},
		},
		DesiredState: swarm.TaskStateReady,
	}
	nodeLists = swarm.Node{
		ID: "NodeStateReady",
		Status: swarm.NodeStatus{
			State: swarm.NodeStateReady,
		},
	}
	swarmServices = containers.SwarmService{
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
		Spec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{
				Name: "agent_stackstate-agent",
				Labels: map[string]string{
					"com.docker.stack.image":     "docker.io/stackstate/stackstate-agent-2-test:stac-12057-swarm-topology",
					"com.docker.stack.namespace": "agent",
				},
			},
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: swarm.ContainerSpec{
					Image: "stackstate/stackstate-agent-2-test:stac-12057-swarm-topology@sha256:1d463af3e8c407e08bff9f6127e4959d5286a25018ec5269bfad5324815eb367",
				},
			},
			Mode: swarm.ServiceMode{
				Replicated: &swarm.ReplicatedService{Replicas: createIntPointer(1)},
			},
		},
		TaskContainers: []*containers.SwarmTask{
			{
				ID:             "qwerty12345",
				Name:           "/agent_stackstate-agent.1.skz8sp5d1y4f64qykw37mf3k2",
				ContainerImage: "stackstate/stackstate-agent-2-test:stac-12057-swarm-topology@sha256:1d463af3e8c407e08bff9f6127e4959d5286a25018ec5269bfad5324815eb367",
				ContainerStatus: swarm.ContainerStatus{
					ContainerID: "a95f48f7f58b9154afa074d541d1bff142611e3a800f78d6be423e82f8178406",
					ExitCode:    0,
					PID:         341,
				},
				ContainerSpec: swarm.ContainerSpec{
					Image: "stackstate/stackstate-agent-2-test:stac-12057-swarm-topology@sha256:1d463af3e8c407e08bff9f6127e4959d5286a25018ec5269bfad5324815eb367",
				},
				DesiredState: swarm.TaskStateRunning,
			},
		},
		DesiredTasks: 1,
		RunningTasks: 1,
	}
)

func TestDockerUtil_getActiveNodes(t *testing.T) {

	mockSwarmServiceClient := &mockSwarmServiceAPIClient{
		nodeList: func() ([]swarm.Node, error) {
			swarmNodes := []swarm.Node{
				{
					ID: "Node-NodeStateDown",
					Status: swarm.NodeStatus{
						State: swarm.NodeStateDown,
					},
				},
				{
					ID: "Node-NodeStateUnknown",
					Status: swarm.NodeStatus{
						State: swarm.NodeStateUnknown,
					},
				},
				{
					ID: "Node-NodeStateReady",
					Status: swarm.NodeStatus{
						State: swarm.NodeStateReady,
					},
				},
				{
					ID: "Node-NodeStateDisconnected",
					Status: swarm.NodeStatus{
						State: swarm.NodeStateDisconnected,
					},
				},
			}
			return swarmNodes, nil
		},
	}

	nodeMap, err := getActiveNodes(nil, mockSwarmServiceClient)
	assert.NoError(t, err)

	expectedNodeMap := map[string]bool{
		"Node-NodeStateReady": true,
	}
	assert.EqualValues(t, expectedNodeMap, nodeMap)
}

func TestDockerUtil_dockerSwarmServices(t *testing.T) {
	// mock the docker API client using mockSwarmServiceAPIClient abd return the mocked Service, Task and Node
	mockSwarmServiceClient := &mockSwarmServiceAPIClient{
		nodeList: func() ([]swarm.Node, error) {
			return []swarm.Node{nodeLists}, nil
		},
		taskList: func() ([]swarm.Task, error) {
			return []swarm.Task{taskLists}, nil
		},
		serviceList: func() ([]swarm.Service, error) {
			return []swarm.Service{serviceLists}, nil
		},
	}
	// call the actual function to get the SwarmServices
	expectedServices, err := dockerSwarmServices(nil, mockSwarmServiceClient)
	assert.NoError(t, err)
	assert.EqualValues(t, expectedServices, []*containers.SwarmService{&swarmServices})
}
