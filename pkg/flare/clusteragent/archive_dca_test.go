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

func setupClusterAgentIPCAddress(t *testing.T, confMock model.Config, URL string) {
	u, err := url.Parse(URL)
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)

	confMock.SetWithoutSource("cmd_host", host)
	confMock.SetWithoutSource("cmd_port", port)
	confMock.SetWithoutSource("cluster_agent.cmd_port", port)
}
