// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build docker

package v3or4

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestGetV4TaskWithTags(t *testing.T) {
	testDataPath := "./testdata/task_with_tags.json"
	dummyECS, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v4/1234-1/taskWithTags", testDataPath),
	)
	require.NoError(t, err)
	ts := dummyECS.Start()
	defer ts.Close()

	client := NewClient(fmt.Sprintf("%s/v4/1234-1", ts.URL), "v4")
	task, err := client.GetTaskWithTags(context.Background())
	require.NoError(t, err)

	require.Equal(t, expected, task)
}

// expected is an expected Task from ./testdata/task.json
var expected = &Task{
	ClusterName:   "arn:aws:ecs:us-east-1:123457279990:cluster/ecs-cluster",
	TaskARN:       "arn:aws:ecs:us-east-1:123457279990:task/ecs-cluster/938f6d263c464aa5985dc67ab7f38a7e",
	KnownStatus:   "RUNNING",
	DesiredStatus: "RUNNING",
	Family:        "my-redis",
	Version:       "1",
	Limits: map[string]float64{
		"CPU":    1,
		"Memory": 2048,
	},
	PullStartedAt:    "2023-11-20T12:09:45.059013479Z",
	PullStoppedAt:    "2023-11-20T12:10:41.166377771Z",
	AvailabilityZone: "us-east-1d",
	LaunchType:       "EC2",
	TaskTags: map[string]string{
		"aws:ecs:cluster":    "ecs-cluster",
		"aws:ecs:launchtype": "EC2",
	},
	EphemeralStorageMetrics: map[string]int64{"Utilized": 2298, "Reserved": 20496},
	Containers: []Container{
		{
			DockerID:      "938f6d263c464aa5985dc67ab7f38a7e-1714341083",
			Name:          "log_router",
			DockerName:    "log_router",
			Image:         "amazon/aws-for-fluent-bit:latest",
			ImageID:       "sha256:ed2bd1c0fa887e59338a8761e040acc495213fd3c1b2be661c44c7158425e6e3",
			DesiredStatus: "RUNNING",
			KnownStatus:   "RUNNING",
			Limits:        map[string]uint64{"CPU": 2},
			CreatedAt:     "2023-11-20T12:10:44.559880428Z",
			StartedAt:     "2023-11-20T12:10:44.559880428Z",
			Type:          "NORMAL",
			LogDriver:     "awslogs",
			LogOptions: map[string]string{
				"awslogs-group":  "aws",
				"awslogs-region": "us-east-1",
				"awslogs-stream": "log_router/log_router/938f6d263c464a",
			},
			ContainerARN: "arn:aws:ecs:us-east-1:601427279990:container/ecs-cluster/938f6d263c464aa59/dc51359e-7f8a",
			Networks: []Network{
				{
					NetworkMode:   "awsvpc",
					IPv4Addresses: []string{"172.31.15.128"},
				},
			},
			Snapshotter: "overlayfs",
		},
		{
			DockerID:   "938f6d263c464aa5985dc67ab7f38a7e-2537586469",
			Name:       "datadog-agent",
			DockerName: "datadog-agent",
			Image:      "public.ecr.aws/datadog/agent:latest",
			ImageID:    "sha256:ba1d175ac08f8241d62c07785cbc6e026310cd2293dc4cf148e05d63655d1297",
			Labels: map[string]string{
				"com.amazonaws.ecs.container-name":          "datadog-agent",
				"com.amazonaws.ecs.task-definition-family":  "my-redis",
				"com.amazonaws.ecs.task-definition-version": "1",
			},
			DesiredStatus: "RUNNING",
			KnownStatus:   "RUNNING",
			Limits:        map[string]uint64{"CPU": 2},
			CreatedAt:     "2023-11-20T12:10:44.404563253Z",
			StartedAt:     "2023-11-20T12:10:44.404563253Z",
			Type:          "NORMAL",
			Health: &HealthStatus{
				Status:   "HEALTHY",
				Since:    "2023-11-20T12:11:16.383262018Z",
				ExitCode: pointer.Ptr(int64(-1)),
			},
			Volumes: []Volume{
				{
					DockerName:  "my-redis-1-dd-sockets",
					Destination: "/var/run/datadog",
				},
			},
			LogDriver: "awslogs",
			LogOptions: map[string]string{
				"awslogs-group":  "aws",
				"awslogs-region": "us-east-1",
				"awslogs-stream": "log_router/datadog-agent/938f6d263c46e",
			},
			ContainerARN: "arn:aws:ecs:us-east-1:601427279990:container/ecs-cluster/938f6d263c464aa/a17c293b-ab52",
			Networks: []Network{
				{
					NetworkMode:   "awsvpc",
					IPv4Addresses: []string{"172.31.115.123"},
				},
			},
			Snapshotter: "overlayfs",
		},
	},
}
