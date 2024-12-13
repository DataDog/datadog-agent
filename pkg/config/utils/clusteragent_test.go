// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	clusterAgentServiceName = "DATADOG_CLUSTER_AGENT"
	clusterAgentServiceHost = clusterAgentServiceName + "_SERVICE_HOST"
	clusterAgentServicePort = clusterAgentServiceName + "_SERVICE_PORT"
)

func TestGetClusterAgentEndpointEmpty(t *testing.T) {
	configmock.New(t)
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.url", "")
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.kubernetes_service_name", "")
	_, err := GetClusterAgentEndpoint()
	require.NotNil(t, err)
}

func TestGetClusterAgentEndpointFromUrl(t *testing.T) {
	configmock.New(t)
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.url", "https://127.0.0.1:8080")
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.kubernetes_service_name", "")
	_, err := GetClusterAgentEndpoint()
	require.Nil(t, err, fmt.Sprintf("%v", err))

	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.url", "https://127.0.0.1")
	_, err = GetClusterAgentEndpoint()
	require.Nil(t, err, fmt.Sprintf("%v", err))

	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.url", "127.0.0.1")
	endpoint, err := GetClusterAgentEndpoint()
	require.Nil(t, err, fmt.Sprintf("%v", err))
	assert.Equal(t, "https://127.0.0.1", endpoint)

	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.url", "127.0.0.1:1234")
	endpoint, err = GetClusterAgentEndpoint()
	require.Nil(t, err, fmt.Sprintf("%v", err))
	assert.Equal(t, "https://127.0.0.1:1234", endpoint)
}

func TestGetClusterAgentEndpointFromUrlInvalid(t *testing.T) {
	configmock.New(t)
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.url", "http://127.0.0.1:8080")
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.kubernetes_service_name", "")
	_, err := GetClusterAgentEndpoint()
	require.NotNil(t, err)

	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.url", "tcp://127.0.0.1:8080")
	_, err = GetClusterAgentEndpoint()
	require.NotNil(t, err)
}

func TestGetClusterAgentEndpointFromKubernetesSvc(t *testing.T) {
	configmock.New(t)
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.url", "")
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	t.Setenv(clusterAgentServiceHost, "127.0.0.1")
	t.Setenv(clusterAgentServicePort, "443")

	endpoint, err := GetClusterAgentEndpoint()
	require.Nil(t, err, fmt.Sprintf("%v", err))
	assert.Equal(t, "https://127.0.0.1:443", endpoint)
}

func TestGetClusterAgentEndpointFromKubernetesSvcEmpty(t *testing.T) {
	configmock.New(t)
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.url", "")
	pkgconfigsetup.Datadog().SetWithoutSource("cluster_agent.kubernetes_service_name", "datadog-cluster-agent")
	t.Setenv(clusterAgentServiceHost, "127.0.0.1")
	t.Setenv(clusterAgentServicePort, "")

	_, err := GetClusterAgentEndpoint()
	require.NotNil(t, err, fmt.Sprintf("%v", err))

	t.Setenv(clusterAgentServiceHost, "")
	t.Setenv(clusterAgentServicePort, "443")
	_, err = GetClusterAgentEndpoint()
	require.NotNil(t, err, fmt.Sprintf("%v", err))
}
