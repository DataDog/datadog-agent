// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

package v2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/testutil"
)

func TestGetTask(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)
	ecsinterface, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v2/metadata", "./testdata/task.json"),
	)
	require.Nil(t, err)

	ts := ecsinterface.Start()
	defer ts.Close()

	expected := &Task{
		ClusterName: "default",
		Containers: []Container{
			{
				Name: "~internal~ecs~pause",
				Limits: map[string]uint64{
					"CPU":    0,
					"Memory": 0,
				},
				ImageID:    "",
				StartedAt:  "2018-02-01T20:55:09.058354915Z",
				DockerName: "ecs-nginx-5-internalecspause-acc699c0cbf2d6d11700",
				Type:       "CNI_PAUSE",
				Image:      "amazon/amazon-ecs-pause:0.1.0",
				Labels: map[string]string{
					"com.amazonaws.ecs.cluster":                 "default",
					"com.amazonaws.ecs.container-name":          "~internal~ecs~pause",
					"com.amazonaws.ecs.task-arn":                "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
					"com.amazonaws.ecs.task-definition-family":  "nginx",
					"com.amazonaws.ecs.task-definition-version": "5",
				},
				KnownStatus:   "RESOURCES_PROVISIONED",
				DesiredStatus: "RESOURCES_PROVISIONED",
				DockerID:      "731a0d6a3b4210e2448339bc7015aaa79bfe4fa256384f4102db86ef94cbbc4c",
				CreatedAt:     "2018-02-01T20:55:08.366329616Z",
				Networks: []Network{
					{
						NetworkMode:   "awsvpc",
						IPv4Addresses: []string{"10.0.2.106"},
					},
				},
			},
			{
				Name: "nginx-curl",
				Limits: map[string]uint64{
					"CPU":    512,
					"Memory": 512,
				},
				ImageID:    "sha256:2e00ae64383cfc865ba0a2ba37f61b50a120d2d9378559dcd458dc0de47bc165",
				StartedAt:  "2018-02-01T20:55:11.064236631Z",
				DockerName: "ecs-nginx-5-nginx-curl-ccccb9f49db0dfe0d901",
				Type:       "NORMAL",
				Image:      "nrdlngr/nginx-curl",
				Labels: map[string]string{
					"com.amazonaws.ecs.cluster":                 "default",
					"com.amazonaws.ecs.container-name":          "nginx-curl",
					"com.amazonaws.ecs.task-arn":                "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
					"com.amazonaws.ecs.task-definition-family":  "nginx",
					"com.amazonaws.ecs.task-definition-version": "5",
				},
				KnownStatus:   "RUNNING",
				DesiredStatus: "RUNNING",
				DockerID:      "43481a6ce4842eec8fe72fc28500c6b52edcc0917f105b83379f88cac1ff3946",
				CreatedAt:     "2018-02-01T20:55:10.554941919Z",
				Networks: []Network{
					{
						NetworkMode:   "awsvpc",
						IPv4Addresses: []string{"10.0.2.106"},
					},
				},
				Ports: []Port{
					{
						ContainerPort: 80,
						Protocol:      "tcp",
					},
				},
			},
		},
		KnownStatus:      "RUNNING",
		TaskARN:          "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
		Family:           "nginx",
		Version:          "5",
		DesiredStatus:    "RUNNING",
		AvailabilityZone: "us-east-2b",
	}

	metadata, err := NewClient(ts.URL).GetTask(ctx)
	assert.Nil(err)
	assert.Equal(expected, metadata)

	select {
	case r := <-ecsinterface.Requests:
		assert.Equal("GET", r.Method)
		assert.Equal("/v2/metadata", r.URL.Path)
	case <-time.After(2 * time.Second):
		assert.FailNow("Timeout on receive channel")
	}
}

func TestGetTaskWithTags(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)
	ecsinterface, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v2/metadata", "./testdata/task_with_tags.json"),
	)
	require.Nil(t, err)

	ts := ecsinterface.Start()
	defer ts.Close()

	expected := &Task{
		ClusterName: "ecs-cluster",
		Containers: []Container{
			{
				Name: "~internal~ecs~pause",
				Limits: map[string]uint64{
					"CPU":    0,
					"Memory": 0,
				},
				ImageID:    "",
				StartedAt:  "2019-10-25T09:36:27.605078427Z",
				DockerName: "ecs-ecs-cluster_redis-awsvpc-12-internalecspause-90cdd286f3e6a6916700",
				Type:       "CNI_PAUSE",
				Image:      "amazon/amazon-ecs-pause:0.1.0",
				Labels: map[string]string{
					"com.amazonaws.ecs.cluster":                 "ecs-cluster",
					"com.amazonaws.ecs.container-name":          "~internal~ecs~pause",
					"com.amazonaws.ecs.task-arn":                "arn:aws:ecs:us-west-1:601427279990:task/ecs-cluster/36a297a28f0f4ad2959801747cfe8703",
					"com.amazonaws.ecs.task-definition-family":  "ecs-cluster_redis-awsvpc",
					"com.amazonaws.ecs.task-definition-version": "12",
				},
				KnownStatus:   "RESOURCES_PROVISIONED",
				DesiredStatus: "RESOURCES_PROVISIONED",
				DockerID:      "4b5455ccf013f78f610c2116cf63a81771ec654a9f63683fb4c22122dbff1986",
				CreatedAt:     "2019-10-25T09:36:24.024505133Z",
				Networks: []Network{
					{
						NetworkMode:   "awsvpc",
						IPv4Addresses: []string{"10.0.2.176"},
					},
				},
			},
			{
				Name: "redis",
				Limits: map[string]uint64{
					"CPU":    0,
					"Memory": 256,
				},
				ImageID:    "sha256:de25a81a5a0b6ff26c82bab404fff5de5bf4bbbc48c833412fb3706077d31134",
				StartedAt:  "2019-10-25T09:36:32.652702457Z",
				DockerName: "ecs-ecs-cluster_redis-awsvpc-12-redis-86fe99b5ffeabffeed01",
				Type:       "NORMAL",
				Image:      "redis",
				Labels: map[string]string{
					"com.amazonaws.ecs.cluster":                 "ecs-cluster",
					"com.amazonaws.ecs.container-name":          "redis",
					"com.amazonaws.ecs.task-arn":                "arn:aws:ecs:us-west-1:601427279990:task/ecs-cluster/36a297a28f0f4ad2959801747cfe8703",
					"com.amazonaws.ecs.task-definition-family":  "ecs-cluster_redis-awsvpc",
					"com.amazonaws.ecs.task-definition-version": "12",
				},
				KnownStatus:   "RUNNING",
				DesiredStatus: "RUNNING",
				DockerID:      "470f831ceac0479b8c6614a7232e707fb24760c350b13ee589dd1d6424315d98",
				CreatedAt:     "2019-10-25T09:36:32.099340865Z",
				Networks: []Network{
					{
						NetworkMode:   "awsvpc",
						IPv4Addresses: []string{"10.0.2.176"},
					},
				},
			},
		},
		KnownStatus:      "RUNNING",
		TaskARN:          "arn:aws:ecs:us-west-1:601427279990:task/ecs-cluster/36a297a28f0f4ad2959801747cfe8703",
		Family:           "ecs-cluster_redis-awsvpc",
		Version:          "12",
		DesiredStatus:    "RUNNING",
		AvailabilityZone: "us-west-1c",
		TaskTags: map[string]string{
			"Creator":             "john.doe",
			"Environment":         "sandbox",
			"Name":                "ecs-cluster-cluster",
			"Team":                "dev-1",
			"aws:ecs:clusterName": "ecs-cluster",
			"aws:ecs:serviceName": "ec2-redis-awspvc",
		},
		ContainerInstanceTags: map[string]string{
			"InstanceType":  "storage",
			"InstanceGroup": "databases",
		},
	}

	metadata, err := NewClient(ts.URL).GetTask(ctx)
	assert.Nil(err)
	assert.Equal(expected, metadata)

	select {
	case r := <-ecsinterface.Requests:
		assert.Equal("GET", r.Method)
		assert.Equal("/v2/metadata", r.URL.Path)
	case <-time.After(2 * time.Second):
		assert.FailNow("Timeout on receive channel")
	}
}

func TestGetContainerStats(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)

	tests := []struct {
		name          string
		fixture       string
		containerID   string
		expectedStats *ContainerStats
		expectedErr   error
	}{
		{
			name:        "net-stats",
			fixture:     "./testdata/container_stats.json",
			containerID: "470f831ceac0479b8c6614a7232e707fb24760c350b13ee589dd1d6424315d98",
			expectedStats: &ContainerStats{
				Timestamp: "2019-10-25T10:07:01.006590487Z",
				CPU: CPUStats{
					System: 3951680000000,
					Usage: CPUUsage{
						Kernelmode: 2260000000,
						Total:      9743590394,
						Usermode:   7450000000,
					},
				},
				Memory: MemStats{
					Details: DetailedMem{
						RSS:     1564672,
						Cache:   65499136,
						PgFault: 430478,
					},
					Limit:    268435456,
					MaxUsage: 139751424,
					Usage:    77254656,
				},
				IO: IOStats{
					BytesPerDeviceAndKind: []OPStat{
						{
							Kind:  "Read",
							Major: 259,
							Minor: 0,
							Value: 12288,
						},
						{
							Kind:  "Write",
							Major: 259,
							Minor: 0,
							Value: 144908288,
						},
						{
							Kind:  "Sync",
							Major: 259,
							Minor: 0,
							Value: 8122368,
						},
						{
							Kind:  "Async",
							Major: 259,
							Minor: 0,
							Value: 136798208,
						},
						{
							Kind:  "Total",
							Major: 259,
							Minor: 0,
							Value: 144920576,
						},
					},
					OPPerDeviceAndKind: []OPStat{
						{
							Kind:  "Read",
							Major: 259,
							Minor: 0,
							Value: 3,
						},
						{
							Kind:  "Write",
							Major: 259,
							Minor: 0,
							Value: 1618,
						},
						{
							Kind:  "Sync",
							Major: 259,
							Minor: 0,
							Value: 514,
						},
						{
							Kind:  "Async",
							Major: 259,
							Minor: 0,
							Value: 1107,
						},
						{
							Kind:  "Total",
							Major: 259,
							Minor: 0,
							Value: 1621,
						},
					},
					ReadBytes:  0,
					WriteBytes: 0,
				},
				Networks: NetStatsMap{
					"eth0": NetStats{
						RxBytes:   163710528,
						RxPackets: 113457,
						TxBytes:   1103607,
						TxPackets: 16969,
					},
				},
			},
		},
		{
			name:        "no-net-stats",
			fixture:     "./testdata/container_stats_empty_net_stats.json",
			containerID: "470f831ceac0479b8c6614a7232e707fb24760c350b13ee589dd1d6424315d98",
			expectedStats: &ContainerStats{
				Timestamp: "2019-10-25T10:07:01.006590487Z",
				CPU: CPUStats{
					System: 3951680000000,
					Usage: CPUUsage{
						Kernelmode: 2260000000,
						Total:      9743590394,
						Usermode:   7450000000,
					},
				},
				Memory: MemStats{
					Details: DetailedMem{
						RSS:     1564672,
						Cache:   65499136,
						PgFault: 430478,
					},
					Limit:    268435456,
					MaxUsage: 139751424,
					Usage:    77254656,
				},
				IO: IOStats{
					BytesPerDeviceAndKind: []OPStat{
						{
							Kind:  "Read",
							Major: 259,
							Minor: 0,
							Value: 12288,
						},
						{
							Kind:  "Write",
							Major: 259,
							Minor: 0,
							Value: 144908288,
						},
						{
							Kind:  "Sync",
							Major: 259,
							Minor: 0,
							Value: 8122368,
						},
						{
							Kind:  "Async",
							Major: 259,
							Minor: 0,
							Value: 136798208,
						},
						{
							Kind:  "Total",
							Major: 259,
							Minor: 0,
							Value: 144920576,
						},
					},
					OPPerDeviceAndKind: []OPStat{
						{
							Kind:  "Read",
							Major: 259,
							Minor: 0,
							Value: 3,
						},
						{
							Kind:  "Write",
							Major: 259,
							Minor: 0,
							Value: 1618,
						},
						{
							Kind:  "Sync",
							Major: 259,
							Minor: 0,
							Value: 514,
						},
						{
							Kind:  "Async",
							Major: 259,
							Minor: 0,
							Value: 1107,
						},
						{
							Kind:  "Total",
							Major: 259,
							Minor: 0,
							Value: 1621,
						},
					},
					ReadBytes:  0,
					WriteBytes: 0,
				},
			},
		},
		{
			name:        "missing-container",
			fixture:     "./testdata/container_stats.json",
			containerID: "470f831ceac0479b8c6614a7232e707fb24760c350b13ee589dd1d6424315d42",
			expectedErr: errors.New("Failed to retrieve container stats for id: 470f831ceac0479b8c6614a7232e707fb24760c350b13ee589dd1d6424315d42"),
		},
	}

	handlerPath := "/v2/stats"

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ecsinterface, err := testutil.NewDummyECS(
				testutil.FileHandlerOption(handlerPath, test.fixture),
			)
			require.Nil(t, err)

			ts := ecsinterface.Start()
			defer ts.Close()

			metadata, err := NewClient(ts.URL).GetContainerStats(ctx, test.containerID)
			assert.Equal(test.expectedStats, metadata)
			assert.Equal(test.expectedErr, err)

			select {
			case r := <-ecsinterface.Requests:
				assert.Equal("GET", r.Method)
				assert.Equal(handlerPath, r.URL.Path)
			case <-time.After(2 * time.Second):
				assert.FailNow("Timeout on receive channel")
			}
		})
	}
}
