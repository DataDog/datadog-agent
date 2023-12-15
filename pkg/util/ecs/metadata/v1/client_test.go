// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

package v1

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/testutil"
)

func TestGetInstance(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)

	ecsinterface, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v1/metadata", "./testdata/instance.json"),
	)
	require.Nil(t, err)

	ts := ecsinterface.Start()
	defer ts.Close()

	expected := &Instance{
		Cluster: "ecs_cluster",
	}

	client := NewClient(ts.URL)
	meta, err := client.GetInstance(ctx)
	assert.Nil(err)
	assert.Equal(expected, meta)

	select {
	case r := <-ecsinterface.Requests:
		assert.Equal("GET", r.Method)
		assert.Equal("/v1/metadata", r.URL.Path)
	case <-time.After(2 * time.Second):
		assert.FailNow("Timeout on receive channel")
	}
}

func TestGetTasks(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)

	ecsinterface, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v1/tasks", "./testdata/tasks.json"),
	)
	require.Nil(t, err)

	ts := ecsinterface.Start()
	defer ts.Close()

	expected := []Task{
		{
			Arn:           "arn:aws:ecs:us-east-1:<aws_account_id>:task/example5-58ff-46c9-ae05-543f8example",
			DesiredStatus: "RUNNING",
			KnownStatus:   "RUNNING",
			Family:        "hello_world",
			Version:       "8",
			Containers: []Container{
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
	}

	client := NewClient(ts.URL)
	tasks, err := client.GetTasks(ctx)
	assert.Nil(err)
	assert.Equal(expected, tasks)

	select {
	case r := <-ecsinterface.Requests:
		assert.Equal("GET", r.Method)
		assert.Equal("/v1/tasks", r.URL.Path)
	case <-time.After(2 * time.Second):
		assert.FailNow("Timeout on receive channel")
	}
}

func TestGetTasksFail(t *testing.T) {
	ctx := context.Background()
	assert := assert.New(t)

	ecsinterface, err := testutil.NewDummyECS(
		testutil.RawHandlerOption("/v1/tasks", ""),
	)
	require.Nil(t, err)

	ts := ecsinterface.Start()
	defer ts.Close()

	var expected []Task
	expectedErr := errors.New("Failed to decode metadata v1 JSON payload to type *v1.Tasks: EOF")

	client := NewClient(ts.URL)
	tasks, err := client.GetTasks(ctx)

	assert.NotNil(err)
	assert.Equal(expectedErr.Error(), err.Error())
	assert.Equal(expected, tasks)

	select {
	case r := <-ecsinterface.Requests:
		assert.Equal("GET", r.Method)
		assert.Equal("/v1/tasks", r.URL.Path)
	case <-time.After(2 * time.Second):
		assert.FailNow("Timeout on receive channel")
	}
}
