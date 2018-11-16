// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package collectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerutil "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
)

func TestECSParseTasks(t *testing.T) {
	ecsExpireFreq := 5 * time.Minute
	expiretest, _ := taggerutil.NewExpire(ecsExpireFreq)
	ecsCollector := &ECSCollector{
		expire:      expiretest,
		clusterName: "test-cluster",
	}

	for nb, tc := range []struct {
		input    ecsutil.TasksV1Response
		expected []*TagInfo
		err      error
	}{
		{
			input:    ecsutil.TasksV1Response{},
			expected: []*TagInfo{},
			err:      nil,
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
			expected: []*TagInfo{
				{
					Source:       "ecs",
					Entity:       "docker://9581a69a761a557fbfce1d0f6745e4af5b9dbfb86b6b2c5c4df156f1a5932ff1",
					HighCardTags: []string{"task_arn:arn:aws:ecs:us-east-1:<aws_account_id>:task/example5-58ff-46c9-ae05-543f8example"},
					LowCardTags:  []string{"ecs_container_name:mysql", "cluster_name:test-cluster", "task_version:8", "task_name:hello_world", "task_family:hello_world"},
				},
				{
					Source:       "ecs",
					Entity:       "docker://bf25c5c5b2d4dba68846c7236e75b6915e1e778d31611e3c6a06831e39814a15",
					HighCardTags: []string{"task_arn:arn:aws:ecs:us-east-1:<aws_account_id>:task/example5-58ff-46c9-ae05-543f8example"},
					LowCardTags:  []string{"ecs_container_name:wordpress", "cluster_name:test-cluster", "task_version:8", "task_name:hello_world", "task_family:hello_world"},
				},
			},
			err: nil,
		},
	} {
		t.Logf("test case %d", nb)
		infos, err := ecsCollector.parseTasks(tc.input, "")
		if len(infos) > 0 {
			require.Len(t, infos, 2)
		}
		for _, item := range infos {
			t.Logf("testing entity %s", item.Entity)
			require.True(t, requireMatchInfo(t, tc.expected, item))
		}
		if tc.err == nil {
			assert.Nil(t, err)
		} else {
			assert.NotNil(t, err)
			assert.Equal(t, tc.err.Error(), err.Error())
		}
	}
}

func TestECSParseTasksTargetting(t *testing.T) {
	ecsExpireFreq := 5 * time.Minute
	expiretest, _ := taggerutil.NewExpire(ecsExpireFreq)
	ecsCollector := &ECSCollector{
		expire: expiretest,
	}

	input := ecsutil.TasksV1Response{
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
	}

	// First run, collect all
	infos, err := ecsCollector.parseTasks(input, "")
	assert.NoError(t, err)
	assert.Len(t, infos, 2)

	// Second run, collect none (all already seen)
	infos, err = ecsCollector.parseTasks(input, "")
	assert.NoError(t, err)
	assert.Len(t, infos, 0)

	// Force a target container ID
	infos, err = ecsCollector.parseTasks(input, "bf25c5c5b2d4dba68846c7236e75b6915e1e778d31611e3c6a06831e39814a15")
	assert.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, "docker://bf25c5c5b2d4dba68846c7236e75b6915e1e778d31611e3c6a06831e39814a15", infos[0].Entity)
}
