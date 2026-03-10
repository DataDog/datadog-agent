// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v2"
)

func TestEnrichmentDataSerialization(t *testing.T) {
	clusterName := "my-cluster"
	data := EnrichmentData{
		Hostname:     "myhost.example.com",
		HostTags:     map[string]string{"env": "prod", "region": "us-east-1"},
		ClusterName:  &clusterName,
		AgentVersion: "7.50.0",
		ConfigValues: map[string]interface{}{
			"dd_url":    "https://app.datadoghq.com",
			"log_level": "info",
		},
		ProcessStartTime: 1700000000,
	}

	yamlBytes, err := yaml.Marshal(data)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Verify YAML field names match what Rust expects
	assert.Contains(t, yamlStr, "hostname:")
	assert.Contains(t, yamlStr, "host_tags:")
	assert.Contains(t, yamlStr, "cluster_name:")
	assert.Contains(t, yamlStr, "agent_version:")
	assert.Contains(t, yamlStr, "config_values:")
	assert.Contains(t, yamlStr, "process_start_time:")
}

func TestEnrichmentDataRoundTrip(t *testing.T) {
	clusterName := "my-cluster"
	bearerToken := "my-token"
	data := EnrichmentData{
		Hostname:     "myhost.example.com",
		HostTags:     map[string]string{"env": "prod", "region": "us-east-1"},
		ClusterName:  &clusterName,
		AgentVersion: "7.50.0",
		ConfigValues: map[string]interface{}{
			"dd_url":    "https://app.datadoghq.com",
			"log_level": "info",
		},
		ProcessStartTime: 1700000000,
		K8sConnectionInfo: &K8sConnectionInfo{
			APIServerURL: "https://kubernetes.default.svc",
			BearerToken:  &bearerToken,
		},
	}

	yamlBytes, err := yaml.Marshal(data)
	require.NoError(t, err)

	var deserialized EnrichmentData
	err = yaml.Unmarshal(yamlBytes, &deserialized)
	require.NoError(t, err)

	assert.Equal(t, "myhost.example.com", deserialized.Hostname)
	assert.Equal(t, "prod", deserialized.HostTags["env"])
	assert.Equal(t, "us-east-1", deserialized.HostTags["region"])
	require.NotNil(t, deserialized.ClusterName)
	assert.Equal(t, "my-cluster", *deserialized.ClusterName)
	assert.Equal(t, "7.50.0", deserialized.AgentVersion)
	assert.Equal(t, "https://app.datadoghq.com", deserialized.ConfigValues["dd_url"])
	assert.Equal(t, "info", deserialized.ConfigValues["log_level"])
	assert.Equal(t, uint64(1700000000), deserialized.ProcessStartTime)

	require.NotNil(t, deserialized.K8sConnectionInfo)
	assert.Equal(t, "https://kubernetes.default.svc", deserialized.K8sConnectionInfo.APIServerURL)
	require.NotNil(t, deserialized.K8sConnectionInfo.BearerToken)
	assert.Equal(t, "my-token", *deserialized.K8sConnectionInfo.BearerToken)
}

func TestEnrichmentDataWithNilOptionalFields(t *testing.T) {
	data := EnrichmentData{
		Hostname:         "host1",
		HostTags:         map[string]string{},
		AgentVersion:     "7.50.0",
		ConfigValues:     map[string]interface{}{},
		ProcessStartTime: 0,
		// ClusterName and K8sConnectionInfo are nil
	}

	yamlBytes, err := yaml.Marshal(data)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// omitempty should omit nil optional fields
	assert.NotContains(t, yamlStr, "cluster_name")
	assert.NotContains(t, yamlStr, "k8s_connection_info")

	// Verify round-trip still works (Rust serde handles missing optional fields as None)
	var deserialized EnrichmentData
	err = yaml.Unmarshal(yamlBytes, &deserialized)
	require.NoError(t, err)

	assert.Equal(t, "host1", deserialized.Hostname)
	assert.Nil(t, deserialized.ClusterName)
	assert.Nil(t, deserialized.K8sConnectionInfo)
}

func TestStaticProvider(t *testing.T) {
	data := EnrichmentData{
		Hostname:         "test-host",
		HostTags:         map[string]string{"env": "test"},
		AgentVersion:     "7.50.0",
		ConfigValues:     map[string]interface{}{"log_level": "debug"},
		ProcessStartTime: 1700000000,
	}

	provider, err := NewStaticProvider(data)
	require.NoError(t, err)

	yamlStr := provider.GetEnrichmentYAML()
	assert.Contains(t, yamlStr, "hostname: test-host")
	assert.Contains(t, yamlStr, "agent_version: 7.50.0")
	assert.Contains(t, yamlStr, "process_start_time: 1700000000")

	// Verify the YAML is valid and can be deserialized back
	var deserialized EnrichmentData
	err = yaml.Unmarshal([]byte(yamlStr), &deserialized)
	require.NoError(t, err)
	assert.Equal(t, "test-host", deserialized.Hostname)
	assert.Equal(t, "test", deserialized.HostTags["env"])
}

func TestStaticProviderNilMaps(t *testing.T) {
	// Verify that nil maps are initialized to empty maps, not serialized as "null"
	data := EnrichmentData{
		Hostname:     "test-host",
		AgentVersion: "7.50.0",
		// HostTags and ConfigValues are nil
	}

	provider, err := NewStaticProvider(data)
	require.NoError(t, err)

	yamlStr := provider.GetEnrichmentYAML()

	// Should contain empty map notation, not "null"
	assert.NotContains(t, yamlStr, "host_tags: null")
	assert.NotContains(t, yamlStr, "config_values: null")

	// Should still be valid for Rust deserialization
	var deserialized EnrichmentData
	err = yaml.Unmarshal([]byte(yamlStr), &deserialized)
	require.NoError(t, err)
	assert.NotNil(t, deserialized.HostTags)
	assert.NotNil(t, deserialized.ConfigValues)
}

func TestStaticProviderImplementsInterface(t *testing.T) {
	data := EnrichmentData{
		Hostname:     "test-host",
		AgentVersion: "7.50.0",
	}

	provider, err := NewStaticProvider(data)
	require.NoError(t, err)

	// Verify StaticProvider implements Provider interface
	var _ Provider = provider
}
