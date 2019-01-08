// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package ecs

import (
	"testing"

	payload "github.com/DataDog/agent-payload/gogen"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/stretchr/testify/assert"
)

var nextTestResponse ecsutil.TasksV1Response

func TestParseTaskResponse(t *testing.T) {
	assert := assert.New(t)
	for nb, tc := range []struct {
		input    ecsutil.TasksV1Response
		expected *payload.ECSMetadataPayload
	}{
		{
			input: ecsutil.TasksV1Response{},
			expected: &payload.ECSMetadataPayload{
				Tasks: []*payload.ECSMetadataPayload_Task{},
			},
		},
		{
			input: ecsutil.TasksV1Response{
				Tasks: []ecsutil.TaskV1{
					{
						Arn:           "arn:aws:ecs:us-east-1:<aws_account_id>:task/example5-58ff-46c9-ae05-543f8example",
						DesiredStatus: "RUNNING",
						KnownStatus:   "RUNNING",
						Family:        "hello_world",
						Version:       "8",
						Containers: []ecsutil.ContainerV1{
							{
								DockerID:   "9581a69a761a557fbfce1d0f6745e4af5b9dbfb86b6b2c5c4df156f1a5932ff1",
								DockerName: "ecs-hello_world-8-mysql-fcae8ac8f9f1d89d8301",
								Name:       "mysql",
							},
							{
								DockerID:   "bf25c5c5b2d4dba68846c7236e75b6915e1e778d31611e3c6a06831e39814a15",
								DockerName: "ecs-hello_world-8-wordpress-e8bfddf9b488dff36c00",
								Name:       "wordpress",
							},
						},
					},
				},
			},
			expected: &payload.ECSMetadataPayload{
				Tasks: []*payload.ECSMetadataPayload_Task{
					{
						Arn:           "arn:aws:ecs:us-east-1:<aws_account_id>:task/example5-58ff-46c9-ae05-543f8example",
						DesiredStatus: "RUNNING",
						KnownStatus:   "RUNNING",
						Family:        "hello_world",
						Version:       "8",
						Containers: []*payload.ECSMetadataPayload_Container{
							{
								DockerId:   "9581a69a761a557fbfce1d0f6745e4af5b9dbfb86b6b2c5c4df156f1a5932ff1",
								DockerName: "ecs-hello_world-8-mysql-fcae8ac8f9f1d89d8301",
								Name:       "mysql",
							},
							{
								DockerId:   "bf25c5c5b2d4dba68846c7236e75b6915e1e778d31611e3c6a06831e39814a15",
								DockerName: "ecs-hello_world-8-wordpress-e8bfddf9b488dff36c00",
								Name:       "wordpress",
							},
						},
					},
				},
			},
		},
	} {
		t.Logf("test case %d", nb)
		nextTestResponse = tc.input
		p := parseTaskResponse(nextTestResponse)
		assert.Equal(tc.expected, p)
	}
}
