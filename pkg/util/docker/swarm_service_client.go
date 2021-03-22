// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
)

// SwarmServiceAPIClient defines API client methods for the swarm services with it's metadata
type SwarmServiceAPIClient interface {
	ServiceList(ctx context.Context, options types.ServiceListOptions) ([]swarm.Service, error)
	TaskList(ctx context.Context, options types.TaskListOptions) ([]swarm.Task, error)
	NodeList(ctx context.Context, options types.NodeListOptions) ([]swarm.Node, error)
}

type mockSwarmServiceAPIClient struct {
	serviceList func() ([]swarm.Service, error)
	taskList    func() ([]swarm.Task, error)
	nodeList    func() ([]swarm.Node, error)
}

func (m *mockSwarmServiceAPIClient) ServiceList(ctx context.Context, options types.ServiceListOptions) ([]swarm.Service, error) {
	return m.serviceList()
}

func (m *mockSwarmServiceAPIClient) TaskList(ctx context.Context, options types.TaskListOptions) ([]swarm.Task, error) {
	return m.taskList()
}

func (m *mockSwarmServiceAPIClient) NodeList(ctx context.Context, options types.NodeListOptions) ([]swarm.Node, error) {
	return m.nodeList()
}
