// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package ecs

import (
	"fmt"
	"net"
	"net/http"

	"testing"

	payload "github.com/DataDog/agent-payload/gogen"
	"github.com/stretchr/testify/assert"
)

var nextTestResponse string

// TODO: ideally this test should use the httptest package
func runServer(t *testing.T, ready chan<- bool, exit <-chan bool) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Satisfy the initial check of URLs
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{\"AvailableCommands\":[\"/license\",\"/v1/metadata\",\"/v1/tasks\"]}"))
	})
	http.HandleFunc("/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(nextTestResponse))
	})

	s := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", DefaultAgentPort)}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		t.Fail()
	}
	go s.Serve(ln)
	ready <- true
	<-exit
	s.Close()
}

func TestGetPayload(t *testing.T) {
	assert := assert.New(t)
	exit := make(chan bool, 1)
	ready := make(chan bool, 1)
	go runServer(t, ready, exit)
	<-ready
	for _, tc := range []struct {
		input    string
		expected *payload.ECSMetadataPayload
		hasError bool
	}{
		{
			input: `{}`,
			expected: &payload.ECSMetadataPayload{
				Tasks: []*payload.ECSMetadataPayload_Task{},
			},
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
			expected: &payload.ECSMetadataPayload{
				Tasks: []*payload.ECSMetadataPayload_Task{
					&payload.ECSMetadataPayload_Task{
						Arn:           "arn:aws:ecs:us-east-1:<aws_account_id>:task/example5-58ff-46c9-ae05-543f8example",
						DesiredStatus: "RUNNING",
						KnownStatus:   "RUNNING",
						Family:        "hello_world",
						Version:       "8",
						Containers: []*payload.ECSMetadataPayload_Container{
							&payload.ECSMetadataPayload_Container{
								DockerId:   "9581a69a761a557fbfce1d0f6745e4af5b9dbfb86b6b2c5c4df156f1a5932ff1",
								DockerName: "ecs-hello_world-8-mysql-fcae8ac8f9f1d89d8301",
								Name:       "mysql",
							},
							&payload.ECSMetadataPayload_Container{
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
		nextTestResponse = tc.input
		p, err := GetPayload()
		assert.NoError(err)
		assert.Equal(tc.expected, p)
	}
	exit <- true
}
