// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package ecs

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// dummyECS allows tests to mock a ECS's responses
type dummyECS struct {
	Requests     chan *http.Request
	TaskListJSON string
}

func newDummyECS() (*dummyECS, error) {
	return &dummyECS{Requests: make(chan *http.Request, 3)}, nil
}

func (d *dummyECS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("dummyECS received %s on %s", r.Method, r.URL.Path)
	d.Requests <- r
	switch r.URL.Path {
	case "/":
		w.Write([]byte(`{"AvailableCommands":["/v1/metadata","/v1/tasks","/license"]}`))
	case "/v1/tasks":
		w.Write([]byte(d.TaskListJSON))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (d *dummyECS) Start() (*httptest.Server, int, error) {
	ts := httptest.NewServer(d)
	ecs_agent_url, err := url.Parse(ts.URL)
	if err != nil {
		return nil, 0, err
	}
	ecs_agent_port, err := strconv.Atoi(ecs_agent_url.Port())
	if err != nil {
		return nil, 0, err
	}
	return ts, ecs_agent_port, nil
}

func TestLocateECSHTTP(t *testing.T) {
	assert := assert.New(t)
	ecsinterface, err := newDummyECS()
	require.Nil(t, err)
	ts, ecs_agent_port, err := ecsinterface.Start()
	defer ts.Close()
	require.Nil(t, err)

	config.Datadog.SetDefault("ecs_agent_url", fmt.Sprintf("http://localhost:%d/", ecs_agent_port))

	util, err := GetUtil()
	assert.Nil(err)
	select {
	case r := <-ecsinterface.Requests:
		assert.Equal("GET", r.Method)
		assert.Equal("/", r.URL.Path)
	case <-time.After(2 * time.Second):
		require.FailNow(t, "Timeout on receive channel")
	}
	for nb, tc := range []struct {
		input    string
		expected TasksV1Response
		err      error
	}{
		{
			input:    "",
			expected: TasksV1Response{},
			err:      errors.New("EOF"),
		},
		{
			input: `{
            "Tasks": [
                {
                  "Arn": "arn:aws:ecs:us-east-1:<aws_account_id>:task/example5-58ff-46c9-ae05-543f8example",
                  "DesiredStatus": "RUNNING",
                  "KnownStatus": "RUNNING",
                  "Family": "hello_world",
                  "Version": "8",
                  "Containers": [
                    {
                      "DockerId": "9581a69a761a557fbfce1d0f6745e4af5b9dbfb86b6b2c5c4df156f1a5932ff1",
                      "DockerName": "ecs-hello_world-8-mysql-fcae8ac8f9f1d89d8301",
                      "Name": "mysql"
                    },
                    {
                      "DockerId": "bf25c5c5b2d4dba68846c7236e75b6915e1e778d31611e3c6a06831e39814a15",
                      "DockerName": "ecs-hello_world-8-wordpress-e8bfddf9b488dff36c00",
                      "Name": "wordpress"
                    }
                  ]
                }
              ]
            }`,
			expected: TasksV1Response{
				Tasks: []TaskV1{
					{
						Arn:           "arn:aws:ecs:us-east-1:<aws_account_id>:task/example5-58ff-46c9-ae05-543f8example",
						DesiredStatus: "RUNNING",
						KnownStatus:   "RUNNING",
						Family:        "hello_world",
						Version:       "8",
						Containers: []ContainerV1{
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
			err: nil,
		},
	} {
		t.Logf("test case %d", nb)
		ecsinterface.TaskListJSON = tc.input
		tasks, err := util.GetTasks()
		assert.Equal(tc.expected, tasks)
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
		assert.Equal("/v1/tasks", r.URL.Path)
	case <-time.After(2 * time.Second):
		assert.FailNow("Timeout on receive channel")
	}
}

func TestGetAgentContainerURLS(t *testing.T) {
	config.Datadog.SetDefault("ecs_agent_container_name", "ecs-agent-custom")
	defer config.Datadog.SetDefault("ecs_agent_container_name", "ecs-agent")

	// Setting mocked data in cache
	nets := make(map[string]*network.EndpointSettings)
	nets["bridge"] = &network.EndpointSettings{IPAddress: "172.17.0.2"}
	nets["foo"] = &network.EndpointSettings{IPAddress: "172.17.0.3"}

	co := types.ContainerJSON{
		Config: &container.Config{
			Hostname: "ip-172-29-167-5",
		},
		ContainerJSONBase: &types.ContainerJSONBase{},
		NetworkSettings: &types.NetworkSettings{
			Networks: nets,
		},
	}
	docker.EnableTestingMode()
	cacheKey := docker.GetInspectCacheKey("ecs-agent-custom", false)
	cache.Cache.Set(cacheKey, co, 10*time.Second)

	agentURLS, err := getAgentContainerURLS()
	assert.NoError(t, err)
	require.Len(t, agentURLS, 3)
	assert.Contains(t, agentURLS, "http://172.17.0.2:51678/")
	assert.Contains(t, agentURLS, "http://172.17.0.3:51678/")
	assert.Equal(t, "http://ip-172-29-167-5:51678/", agentURLS[2])
}
