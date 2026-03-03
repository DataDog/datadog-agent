// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
)

// fakev2EcsClient is a fake implementation of v2.Client for testing
type fakev2EcsClient struct {
	mockGetTask func(context.Context) (*v2.Task, error)
}

func (c *fakev2EcsClient) GetTask(ctx context.Context) (*v2.Task, error) {
	return c.mockGetTask(ctx)
}

func (c *fakev2EcsClient) GetTaskWithTags(_ context.Context) (*v2.Task, error) {
	return nil, errors.New("unimplemented")
}

func (c *fakev2EcsClient) GetContainerStats(_ context.Context, _ string) (*v2.ContainerStats, error) {
	return nil, errors.New("unimplemented")
}

// getSampleV2Task returns a sample V2 task for testing
func getSampleV2Task() *v2.Task {
	return &v2.Task{
		ClusterName:      "default",
		TaskARN:          "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
		Family:           "nginx",
		Version:          "5",
		DesiredStatus:    "RUNNING",
		KnownStatus:      "RUNNING",
		AvailabilityZone: "us-east-2b",
		Containers: []v2.Container{
			{
				DockerID:   "731a0d6a3b4210e2448339bc7015aaa79bfe4fa256384f4102db86ef94cbbc4c",
				Name:       "~internal~ecs~pause",
				DockerName: "ecs-nginx-5-internalecspause-acc699c0cbf2d6d11700",
				Image:      "amazon/amazon-ecs-pause:0.1.0",
				Labels: map[string]string{
					"com.amazonaws.ecs.cluster":                "default",
					"com.amazonaws.ecs.container-name":         "~internal~ecs~pause",
					"com.amazonaws.ecs.task-arn":               "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
					"com.amazonaws.ecs.task-definition-family": "nginx",
				},
				DesiredStatus: "RESOURCES_PROVISIONED",
				KnownStatus:   "RESOURCES_PROVISIONED",
				CreatedAt:     "2018-02-01T20:55:08.366329616Z",
				StartedAt:     "2018-02-01T20:55:09.058354915Z",
				Networks: []v2.Network{
					{
						NetworkMode:   "awsvpc",
						IPv4Addresses: []string{"10.0.2.106"},
					},
				},
			},
			{
				DockerID:   "43481a6ce4842eec8fe72fc28500c6b52edcc0917f105b83379f88cac1ff3946",
				Name:       "nginx-curl",
				DockerName: "ecs-nginx-5-nginx-curl-ccccb9f49db0dfe0d901",
				Image:      "nrdlngr/nginx-curl",
				ImageID:    "sha256:2e00ae64383cfc865ba0a2ba37f61b50a120d2d9378559dcd458dc0de47bc165",
				Labels: map[string]string{
					"com.amazonaws.ecs.cluster":                "default",
					"com.amazonaws.ecs.container-name":         "nginx-curl",
					"com.amazonaws.ecs.task-arn":               "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
					"com.amazonaws.ecs.task-definition-family": "nginx",
				},
				DesiredStatus: "RUNNING",
				KnownStatus:   "RUNNING",
				CreatedAt:     "2018-02-01T20:55:10.554941919Z",
				StartedAt:     "2018-02-01T20:55:11.064236631Z",
				Networks: []v2.Network{
					{
						NetworkMode:   "awsvpc",
						IPv4Addresses: []string{"10.0.2.106"},
					},
				},
			},
		},
	}
}

// TestParseTaskFromV2Endpoint tests the parseTaskFromV2Endpoint method
func TestParseTaskFromV2Endpoint(t *testing.T) {
	task := getSampleV2Task()

	collector := &collector{
		metaV2: &fakev2EcsClient{
			mockGetTask: func(_ context.Context) (*v2.Task, error) {
				return task, nil
			},
		},
		actualLaunchType: workloadmeta.ECSLaunchTypeFargate,
		deploymentMode:   deploymentModeSidecar,
		seen:             make(map[workloadmeta.EntityID]struct{}),
	}

	events, err := collector.parseTaskFromV2Endpoint(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, events)

	// Should have 1 task event + 2 container events
	assert.Len(t, events, 3)

	// Verify task event
	var taskEvent *workloadmeta.ECSTask
	var containerCount int
	for _, event := range events {
		assert.Equal(t, workloadmeta.EventTypeSet, event.Type)
		assert.Equal(t, workloadmeta.SourceRuntime, event.Source)

		if ecsTask, ok := event.Entity.(*workloadmeta.ECSTask); ok {
			taskEvent = ecsTask
		} else if _, ok := event.Entity.(*workloadmeta.Container); ok {
			containerCount++
		}
	}

	require.NotNil(t, taskEvent)
	assert.Equal(t, "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3", taskEvent.ID)
	assert.Equal(t, "default", taskEvent.ClusterName)
	assert.Equal(t, "nginx", taskEvent.Family)
	assert.Equal(t, "5", taskEvent.Version)
	assert.Equal(t, workloadmeta.ECSLaunchTypeFargate, taskEvent.LaunchType)
	assert.Equal(t, "us-east-2", taskEvent.Region)
	assert.Equal(t, "012345678910", taskEvent.AWSAccountID)
	assert.Equal(t, "us-east-2b", taskEvent.AvailabilityZone)
	assert.Len(t, taskEvent.Containers, 2)
	assert.Equal(t, 2, containerCount)
}

// TestParseTaskFromV2EndpointError tests error handling in parseTaskFromV2Endpoint
func TestParseTaskFromV2EndpointError(t *testing.T) {
	collector := &collector{
		metaV2: &fakev2EcsClient{
			mockGetTask: func(_ context.Context) (*v2.Task, error) {
				return nil, errors.New("connection refused")
			},
		},
	}

	events, err := collector.parseTaskFromV2Endpoint(context.Background())
	require.Error(t, err)
	assert.Nil(t, events)
	assert.Contains(t, err.Error(), "connection refused")
}

// TestParseV2TaskWithLaunchTypeFargate tests parsing a V2 task with Fargate launch type
func TestParseV2TaskWithLaunchTypeFargate(t *testing.T) {
	task := getSampleV2Task()

	collector := &collector{
		actualLaunchType: workloadmeta.ECSLaunchTypeFargate,
		deploymentMode:   deploymentModeSidecar,
		seen:             make(map[workloadmeta.EntityID]struct{}),
	}

	events := collector.parseV2TaskWithLaunchType(task)
	require.NotEmpty(t, events)

	// Check that Fargate launch type is set correctly
	for _, event := range events {
		if ecsTask, ok := event.Entity.(*workloadmeta.ECSTask); ok {
			assert.Equal(t, workloadmeta.ECSLaunchTypeFargate, ecsTask.LaunchType)
		}
		if container, ok := event.Entity.(*workloadmeta.Container); ok {
			assert.Equal(t, workloadmeta.ContainerRuntimeECSFargate, container.Runtime)
		}
	}
}

// TestParseV2TaskWithLaunchTypeEC2 tests parsing a V2 task with EC2 launch type
func TestParseV2TaskWithLaunchTypeEC2(t *testing.T) {
	task := getSampleV2Task()

	collector := &collector{
		actualLaunchType: workloadmeta.ECSLaunchTypeEC2,
		deploymentMode:   deploymentModeDaemon,
		seen:             make(map[workloadmeta.EntityID]struct{}),
	}

	events := collector.parseV2TaskWithLaunchType(task)
	require.NotEmpty(t, events)

	// Check that EC2 launch type is set and runtime is empty for containers
	for _, event := range events {
		assert.Equal(t, workloadmeta.SourceNodeOrchestrator, event.Source)
		if ecsTask, ok := event.Entity.(*workloadmeta.ECSTask); ok {
			assert.Equal(t, workloadmeta.ECSLaunchTypeEC2, ecsTask.LaunchType)
		}
		if container, ok := event.Entity.(*workloadmeta.Container); ok {
			assert.Empty(t, container.Runtime)
		}
	}
}

// TestParseV2TaskContainers tests the parseV2TaskContainers method
func TestParseV2TaskContainers(t *testing.T) {
	task := getSampleV2Task()

	collector := &collector{
		actualLaunchType: workloadmeta.ECSLaunchTypeFargate,
		deploymentMode:   deploymentModeSidecar,
	}

	seen := make(map[workloadmeta.EntityID]struct{})
	taskContainers, containerEvents := collector.parseV2TaskContainers(task, seen)

	// Verify orchestrator containers
	require.Len(t, taskContainers, 2)
	assert.Equal(t, "731a0d6a3b4210e2448339bc7015aaa79bfe4fa256384f4102db86ef94cbbc4c", taskContainers[0].ID)
	assert.Equal(t, "~internal~ecs~pause", taskContainers[0].Name)
	assert.Equal(t, "43481a6ce4842eec8fe72fc28500c6b52edcc0917f105b83379f88cac1ff3946", taskContainers[1].ID)
	assert.Equal(t, "nginx-curl", taskContainers[1].Name)

	// Verify container events
	require.Len(t, containerEvents, 2)
	for _, event := range containerEvents {
		assert.Equal(t, workloadmeta.EventTypeSet, event.Type)
		assert.Equal(t, workloadmeta.SourceRuntime, event.Source)

		container, ok := event.Entity.(*workloadmeta.Container)
		require.True(t, ok)
		assert.Equal(t, workloadmeta.ContainerRuntimeECSFargate, container.Runtime)

		// Check network IPs
		if container.ID == "43481a6ce4842eec8fe72fc28500c6b52edcc0917f105b83379f88cac1ff3946" {
			assert.Equal(t, "10.0.2.106", container.NetworkIPs["awsvpc"])
			assert.Equal(t, "nrdlngr/nginx-curl", container.Image.Name)
			assert.True(t, container.State.Running)
			assert.Equal(t, workloadmeta.ContainerStatusRunning, container.State.Status)
		}
	}

	// Verify seen map is populated
	assert.Len(t, seen, 2)
}

// TestParseV2TaskWithStoppedStatus tests that stopped tasks are filtered out
func TestParseV2TaskWithStoppedStatus(t *testing.T) {
	task := getSampleV2Task()
	task.KnownStatus = workloadmeta.ECSTaskKnownStatusStopped

	collector := &collector{
		actualLaunchType: workloadmeta.ECSLaunchTypeFargate,
		deploymentMode:   deploymentModeSidecar,
		seen:             make(map[workloadmeta.EntityID]struct{}),
	}

	events := collector.parseV2TaskWithLaunchType(task)
	assert.Empty(t, events, "Stopped tasks should not generate events")
}
