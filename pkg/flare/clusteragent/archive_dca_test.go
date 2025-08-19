// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package clusteragent

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/flare"
)

func TestGetAutoscalerList(t *testing.T) {
	mockResponse := map[string]any{
		"PodAutoscalers": []interface{}{
			map[string]any{
				"name":      "test-dpa",
				"namespace": "ns",
			},
		},
	}

	ipcMock := ipcmock.New(t)

	// Create test server that responds to /autoscaler-list path
	s := ipcMock.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/autoscaler-list" {
			out, _ := json.Marshal(mockResponse)
			w.Write(out)
		}
	}))

	setupClusterAgentIPCAddress(t, configmock.New(t), s.URL)

	content, err := getDCAAutoscalerList(&flare.RemoteFlareProvider{IPC: ipcMock})
	require.NoError(t, err)

	// Parse the JSON response
	var flareOutput map[string]any
	err = json.Unmarshal(content, &flareOutput)
	require.NoError(t, err, "Failed to unmarshal response JSON")

	assert.Equal(t, mockResponse, flareOutput, "The flare output should match what was sent")
}

func TestGetDCALocalAutoscalingWorkloadList(t *testing.T) {
	mockResponse := map[string]any{
		"LocalAutoscalingWorkloadEntities": []interface{}{
			map[string]any{
				"Datapoints(PodLevel)": 1,
				"MetricName":           "container.memory.usage",
				"Namespace":            "kube-system",
				"PodOwner":             "kube-dns",
			},
			map[string]any{
				"Datapoints(PodLevel)": 2,
				"MetricName":           "container.cpu.usage",
				"Namespace":            "workload-notesapp",
				"PodOwner":             "notes-app-deployment",
			},
		},
	}
	ipcMock := ipcmock.New(t)
	// Create test server that responds to /local-autoscaling-check path
	s := ipcMock.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/local-autoscaling-check" {
			out, _ := json.Marshal(mockResponse)
			w.Write(out)
		}
	}))
	defer s.Close()

	setupClusterAgentIPCAddress(t, configmock.New(t), s.URL)

	content, err := getDCALocalAutoscalingWorkloadList(&flare.RemoteFlareProvider{IPC: ipcMock})
	require.NoError(t, err)

	// Parse the JSON response
	var flareOutput map[string]any
	err = json.Unmarshal(content, &flareOutput)
	require.NoError(t, err, "Failed to unmarshal response JSON")

	// Verify the structure and content more specifically
	entities, ok := flareOutput["LocalAutoscalingWorkloadEntities"].([]interface{})
	require.True(t, ok, "LocalAutoscalingWorkloadEntities should be an array")
	assert.Len(t, entities, 2, "Should have 2 entities")

	// Check first entity structure - note: JSON unmarshaling converts numbers to float64
	firstEntity, ok := entities[0].(map[string]interface{})
	require.True(t, ok, "First entity should be a map")
	assert.Equal(t, float64(1), firstEntity["Datapoints(PodLevel)"], "Should have correct datapoints")
	assert.Equal(t, "container.memory.usage", firstEntity["MetricName"], "Should have correct metric name")
	assert.Equal(t, "kube-system", firstEntity["Namespace"], "Should have correct namespace")
	assert.Equal(t, "kube-dns", firstEntity["PodOwner"], "Should have correct pod owner")

	// Check second entity
	secondEntity, ok := entities[1].(map[string]interface{})
	require.True(t, ok, "Second entity should be a map")
	assert.Equal(t, float64(2), secondEntity["Datapoints(PodLevel)"], "Should have correct datapoints")
	assert.Equal(t, "container.cpu.usage", secondEntity["MetricName"], "Should have correct metric name")
	assert.Equal(t, "workload-notesapp", secondEntity["Namespace"], "Should have correct namespace")
	assert.Equal(t, "notes-app-deployment", secondEntity["PodOwner"], "Should have correct pod owner")
}

func TestGetDCALocalAutoscalingWorkloadListError(t *testing.T) {
	ipcMock := ipcmock.New(t)

	// Create test server that returns an error
	s := ipcMock.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/local-autoscaling-check" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))
		}
	}))
	defer s.Close()

	setupClusterAgentIPCAddress(t, configmock.New(t), s.URL)

	_, err := getDCALocalAutoscalingWorkloadList(&flare.RemoteFlareProvider{IPC: ipcMock})
	assert.Error(t, err, "Should return an error when server responds with error")
}

func setupClusterAgentIPCAddress(t *testing.T, confMock model.Config, URL string) {
	u, err := url.Parse(URL)
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)

	confMock.SetWithoutSource("cmd_host", host)
	confMock.SetWithoutSource("cmd_port", port)
	confMock.SetWithoutSource("cluster_agent.cmd_port", port)
}
