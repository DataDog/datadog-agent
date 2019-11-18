// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package ecs

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskMetadata(t *testing.T) {
	assert := assert.New(t)
	ecsinterface, err := testutil.NewDummyECS()
	require.Nil(t, err)
	ts, _, err := ecsinterface.Start()
	defer ts.Close()
	require.Nil(t, err)
	mockedMetadataURL := fmt.Sprintf("%s/v2/metadata", ts.URL)

	for nb, tc := range []struct {
		input    string
		expected metadata.TaskMetadata
		err      error
	}{
		{
			input: `{
				"Cluster": "default",
				"TaskARN": "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
				"Family": "nginx",
				"Revision": "5",
				"DesiredStatus": "RUNNING",
				"KnownStatus": "RUNNING",
				"Containers": [
				  {
					"DockerId": "731a0d6a3b4210e2448339bc7015aaa79bfe4fa256384f4102db86ef94cbbc4c",
					"Name": "~internal~ecs~pause",
					"DockerName": "ecs-nginx-5-internalecspause-acc699c0cbf2d6d11700",
					"Image": "amazon/amazon-ecs-pause:0.1.0",
					"ImageID": "",
					"Labels": {
					  "com.amazonaws.ecs.cluster": "default",
					  "com.amazonaws.ecs.container-name": "~internal~ecs~pause",
					  "com.amazonaws.ecs.task-arn": "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
					  "com.amazonaws.ecs.task-definition-family": "nginx",
					  "com.amazonaws.ecs.task-definition-version": "5"
					},
					"DesiredStatus": "RESOURCES_PROVISIONED",
					"KnownStatus": "RESOURCES_PROVISIONED",
					"Limits": {
					  "CPU": 0,
					  "Memory": 0
					},
					"CreatedAt": "2018-02-01T20:55:08.366329616Z",
					"StartedAt": "2018-02-01T20:55:09.058354915Z",
					"Type": "CNI_PAUSE",
					"Networks": [
					  {
						"NetworkMode": "awsvpc",
						"IPv4Addresses": [
						  "10.0.2.106"
						]
					  }
					]
				  },
				  {
					"DockerId": "43481a6ce4842eec8fe72fc28500c6b52edcc0917f105b83379f88cac1ff3946",
					"Name": "nginx-curl",
					"DockerName": "ecs-nginx-5-nginx-curl-ccccb9f49db0dfe0d901",
					"Image": "nrdlngr/nginx-curl",
					"ImageID": "sha256:2e00ae64383cfc865ba0a2ba37f61b50a120d2d9378559dcd458dc0de47bc165",
					"Labels": {
					  "com.amazonaws.ecs.cluster": "default",
					  "com.amazonaws.ecs.container-name": "nginx-curl",
					  "com.amazonaws.ecs.task-arn": "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
					  "com.amazonaws.ecs.task-definition-family": "nginx",
					  "com.amazonaws.ecs.task-definition-version": "5"
					},
					"DesiredStatus": "RUNNING",
					"KnownStatus": "RUNNING",
					"Limits": {
					  "CPU": 512,
					  "Memory": 512
					},
					"CreatedAt": "2018-02-01T20:55:10.554941919Z",
					"StartedAt": "2018-02-01T20:55:11.064236631Z",
					"Type": "NORMAL",
					"Networks": [
					  {
						"NetworkMode": "awsvpc",
						"IPv4Addresses": [
						  "10.0.2.106"
						]
					  }
					],
					"Ports": [
						{
							"ContainerPort": 80,
							"Protocol": "tcp"
						}
					]
				  }
				],
				"PullStartedAt": "2018-02-01T20:55:09.372495529Z",
				"PullStoppedAt": "2018-02-01T20:55:10.552018345Z",
				"AvailabilityZone": "us-east-2b"
			  }`,
			expected: metadata.TaskMetadata{
				ClusterName: "default",
				Containers: []metadata.Container{
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
						Networks: []metadata.Network{
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
						Networks: []metadata.Network{
							{
								NetworkMode:   "awsvpc",
								IPv4Addresses: []string{"10.0.2.106"},
							},
						},
						Ports: []metadata.Port{
							{
								ContainerPort: 80,
								Protocol:      "tcp",
							},
						},
					},
				},
				KnownStatus:   "RUNNING",
				TaskARN:       "arn:aws:ecs:us-east-2:012345678910:task/9781c248-0edd-4cdb-9a93-f63cb662a5d3",
				Family:        "nginx",
				Version:       "5",
				DesiredStatus: "RUNNING",
			},
			err: nil,
		},
	} {
		t.Logf("test case %d", nb)
		ecsinterface.MetadataJSON = tc.input
		metadata, err := GetTaskMetadataWithURL(mockedMetadataURL)
		assert.Equal(tc.expected, metadata)
		if tc.err == nil {
			assert.Nil(err)
		} else {
			assert.NotNil(err)
			assert.Equal(tc.err.Error(), err.Error())
		}
	}
	select {
	case r := <-ecsinterface.Requests:
		assert.Equal("GET", r.Method)
		assert.Equal("/v2/metadata", r.URL.Path)
	case <-time.After(2 * time.Second):
		assert.FailNow("Timeout on receive channel")
	}
}

func TestParseContainerNetworkAddresses(t *testing.T) {
	ports := []metadata.Port{
		{
			ContainerPort: 80,
			Protocol:      "tcp",
		},
		{
			ContainerPort: 7000,
			Protocol:      "udp",
		},
	}
	networks := []metadata.Network{
		{
			NetworkMode:   "awsvpc",
			IPv4Addresses: []string{"10.0.2.106"},
		},
		{
			NetworkMode:   "awsvpc",
			IPv4Addresses: []string{"10.0.2.107"},
		},
	}
	expectedOutput := []containers.NetworkAddress{
		{
			IP:       net.ParseIP("10.0.2.106"),
			Port:     80,
			Protocol: "tcp",
		},
		{
			IP:       net.ParseIP("10.0.2.106"),
			Port:     7000,
			Protocol: "udp",
		},
		{
			IP:       net.ParseIP("10.0.2.107"),
			Port:     80,
			Protocol: "tcp",
		},
		{
			IP:       net.ParseIP("10.0.2.107"),
			Port:     7000,
			Protocol: "udp",
		},
	}
	result := metadata.ParseECSContainerNetworkAddresses(ports, networks, "mycontainer")
	assert.Equal(t, expectedOutput, result)
}
