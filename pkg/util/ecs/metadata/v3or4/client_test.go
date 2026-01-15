// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build docker

package v3or4

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

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

	client := NewClient(ts.URL+"/v4/1234-1", "v4")
	task, err := client.GetTaskWithTags(context.Background())
	require.NoError(t, err)

	require.Equal(t, expected, task)
}

func TestGetV4TaskWithTagsWithoutRetryWithDelay(t *testing.T) {
	testDataPath := "./testdata/task_with_tags.json"
	dummyECS, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v4/1234-1/taskWithTags", testDataPath),
		testutil.FileHandlerDelayOption("/v4/1234-1/taskWithTags", 1500*time.Millisecond),
	)
	require.NoError(t, err)
	ts := dummyECS.Start()

	client := NewClient(ts.URL+"/v4/1234-1", "v4")
	task, err := client.GetTaskWithTags(context.Background())

	ts.Close()

	// default timeout is 1000ms while the delay is 1.5s
	require.True(t, os.IsTimeout(err))
	require.Nil(t, task)
	require.Equal(t, uint64(1), dummyECS.RequestCount.Load())
}

func TestGetV4TaskWithTagsWithRetryWithDelay(t *testing.T) {
	testDataPath := "./testdata/task_with_tags.json"
	dummyECS, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v4/1234-1/taskWithTags", testDataPath),
		testutil.FileHandlerDelayOption("/v4/1234-1/taskWithTags", 1500*time.Millisecond),
	)
	require.NoError(t, err)
	ts := dummyECS.Start()

	c := NewClient(
		ts.URL+"/v4/1234-1",
		"v4",
		WithTryOption(100*time.Millisecond, 2*time.Second, func(d time.Duration) time.Duration { return 2 * d }),
	)

	task, err := c.GetTaskWithTags(context.Background())

	ts.Close()

	require.NoError(t, err)
	require.Equal(t, expected, task)
	// 2 requests: 1 initial request + 1 retry and server delay is 1.5s
	// 1st request failed: request timeout is 1s
	// 2nd request succeed: request timeout is 2s
	require.Equal(t, uint64(2), dummyECS.RequestCount.Load())
}

func TestGetContainerStats(t *testing.T) {
	ctx := context.Background()
	handlerPath := "/v4/1234-1/task/stats"

	tests := []struct {
		name        string
		fixture     string
		containerID string
		expected    *ContainerStatsV4
		expectedErr error
	}{
		{
			name:        "net-stats",
			fixture:     "./testdata/task_stats.json",
			containerID: "2207f63945be40abb2d7bca5a2661cfd-0911415269",
			expected: &ContainerStatsV4{
				Timestamp: "2025-10-24T21:33:07.765945058Z",
				CPU: CPUStats{
					Usage:  CPUUsage{Total: 181104000, Usermode: 113190000, Kernelmode: 67914000},
					System: 276134970000000,
				},
				Memory: MemStats{
					Details: DetailedMem{PgFault: 9884},
					Limit:   18446744073709551615,
					Usage:   42827776,
				},
				IO: IOStats{
					BytesPerDeviceAndKind: []OPStat{
						{Major: 259, Minor: 0, Kind: "read", Value: 1875968},
						{Major: 259, Minor: 0, Kind: "write", Value: 0},
						{Major: 252, Minor: 0, Kind: "read", Value: 1875968},
						{Major: 252, Minor: 0, Kind: "write", Value: 0},
						{Major: 259, Minor: 1, Kind: "read", Value: 7593984},
						{Major: 259, Minor: 1, Kind: "write", Value: 28672},
					},
				},
				Networks: NetStatsMap{
					"eth0": {RxBytes: 163710528, RxPackets: 113457, TxBytes: 1103607, TxPackets: 16969},
				},
			},
		},
		{
			name:        "no-net-stats",
			fixture:     "./testdata/task_stats_empty_net_stats.json",
			containerID: "2207f63945be40abb2d7bca5a2661cfd-0911415269",
			expected: &ContainerStatsV4{
				Timestamp: "2025-10-24T21:33:07.765945058Z",
				CPU: CPUStats{
					Usage:  CPUUsage{Total: 181104000, Usermode: 113190000, Kernelmode: 67914000},
					System: 276134970000000,
				},
				Memory: MemStats{
					Details: DetailedMem{PgFault: 9884},
					Limit:   18446744073709551615,
					Usage:   42827776,
				},
				IO: IOStats{
					BytesPerDeviceAndKind: []OPStat{
						{Major: 259, Minor: 0, Kind: "read", Value: 1875968},
						{Major: 259, Minor: 0, Kind: "write", Value: 0},
						{Major: 252, Minor: 0, Kind: "read", Value: 1875968},
						{Major: 252, Minor: 0, Kind: "write", Value: 0},
						{Major: 259, Minor: 1, Kind: "read", Value: 7593984},
						{Major: 259, Minor: 1, Kind: "write", Value: 28672},
					},
				},
			},
		},
		{
			name:        "missing-container",
			fixture:     "./testdata/task_stats.json",
			containerID: "470f831ceac0479b8c6614a7232e707fb24760c350b13ee589dd1d6424315d42",
			expectedErr: errors.New("Failed to retrieve container stats for id: 470f831ceac0479b8c6614a7232e707fb24760c350b13ee589dd1d6424315d42"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dummyECS, err := testutil.NewDummyECS(
				testutil.FileHandlerOption(handlerPath, tt.fixture),
			)
			require.NoError(t, err)
			ts := dummyECS.Start()
			defer ts.Close()

			client := NewClient(ts.URL+"/v4/1234-1", "v4")
			stats, err := client.GetContainerStats(ctx, tt.containerID)

			if tt.expectedErr != nil {
				require.EqualError(t, err, tt.expectedErr.Error())
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, stats)

			select {
			case r := <-dummyECS.Requests:
				require.Equal(t, "GET", r.Method)
				require.Equal(t, handlerPath, r.URL.Path)
			case <-time.After(2 * time.Second):
				t.Fatalf("timeout waiting for request")
			}
		})
	}
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
