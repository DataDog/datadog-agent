// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestParseDiscoveryResult_TwoConfigs(t *testing.T) {
	payload := `[
		{"check_name": "redis", "instances": [{"host": "10.0.0.1"}], "init_config": {"foo": 1}},
		{"name": "redis", "instances": [{"host": "10.0.0.2"}]}
	]`
	configs, err := parseDiscoveryResult("redis", payload)
	require.NoError(t, err)
	require.Len(t, configs, 2)
	assert.Equal(t, "redis", configs[0].Name)
	assert.Equal(t, "redis", configs[1].Name)
	assert.Len(t, configs[0].Instances, 1)
	assert.JSONEq(t, `{"foo":1}`, string(configs[0].InitConfig))
	// Empty init_config defaults to "{}" so check loaders don't trip over an
	// empty document.
	assert.Equal(t, integration.Data(`{}`), configs[1].InitConfig)
}

func TestParseDiscoveryResult_DefaultsToIntegrationName(t *testing.T) {
	payload := `[{"instances":[{"host":"x"}]}]`
	configs, err := parseDiscoveryResult("krakend", payload)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, "krakend", configs[0].Name)
}

func TestParseDiscoveryResult_CheckNameWinsOverName(t *testing.T) {
	payload := `[{"check_name":"override","name":"alias","instances":[{}]}]`
	configs, err := parseDiscoveryResult("redis", payload)
	require.NoError(t, err)
	require.Len(t, configs, 1)
	assert.Equal(t, "override", configs[0].Name)
}

func TestParseDiscoveryResult_EmptyArray(t *testing.T) {
	configs, err := parseDiscoveryResult("redis", `[]`)
	require.NoError(t, err)
	assert.Nil(t, configs)
}

func TestParseDiscoveryResult_InvalidJSON(t *testing.T) {
	_, err := parseDiscoveryResult("redis", `not-json`)
	require.Error(t, err)
}

// TestMarshalService_PrefersBridgeOverOtherNetworks: with multiple networks
// the bridge IP wins (matches %%host%%'s getFallbackHost).
func TestMarshalService_PrefersBridgeOverOtherNetworks(t *testing.T) {
	svc := &fakeService{
		id:    "docker://abc",
		hosts: map[string]string{"main": "1.2.3.4", "bridge": "10.0.0.1"},
		ports: nil,
	}
	jsonStr, ok, err := marshalService(svc)
	require.NoError(t, err)
	require.True(t, ok)
	var got discoveryService
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &got))
	assert.Equal(t, "docker://abc", got.ID)
	assert.Equal(t, "10.0.0.1", got.Host)
	// Empty port list still serializes as an empty array, not null, so the
	// Python side gets a stable shape.
	assert.NotNil(t, got.Ports)
	assert.Empty(t, got.Ports)
}

// TestMarshalService_SingleNetworkUsesIt: a service with exactly one network
// uses that network's IP, matching the %%host%% single-network fallback.
func TestMarshalService_SingleNetworkUsesIt(t *testing.T) {
	svc := &fakeService{
		id:    "docker://abc",
		hosts: map[string]string{"main": "1.2.3.4"},
		ports: nil,
	}
	jsonStr, ok, err := marshalService(svc)
	require.NoError(t, err)
	require.True(t, ok)
	var got discoveryService
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &got))
	assert.Equal(t, "1.2.3.4", got.Host)
}

// TestMarshalService_MultiNetworkWithoutBridge_NotOK: %%host%%'s policy
// refuses to guess between equally-valid IPs when no bridge is present, so
// we treat it as a transient failure (returns ok=false, no error).
func TestMarshalService_MultiNetworkWithoutBridge_NotOK(t *testing.T) {
	svc := &fakeService{
		id:    "docker://abc",
		hosts: map[string]string{"netA": "1.2.3.4", "netB": "5.6.7.8"},
	}
	_, ok, err := marshalService(svc)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMarshalService_NoHostReturnsNotOK(t *testing.T) {
	svc := &fakeService{id: "docker://abc", hosts: map[string]string{}}
	_, ok, err := marshalService(svc)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestMarshalService_PortsRoundTrip(t *testing.T) {
	svc := &fakeService{
		id:    "docker://abc",
		hosts: map[string]string{"main": "1.2.3.4"},
		ports: []servicePort{{Name: "http", Port: 8080}, {Name: "metrics", Port: 9090}},
	}
	jsonStr, ok, err := marshalService(svc)
	require.NoError(t, err)
	require.True(t, ok)
	var got discoveryService
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &got))
	expected := []discoveryPort{{Number: 8080, Name: "http"}, {Number: 9090, Name: "metrics"}}
	assert.True(t, reflect.DeepEqual(expected, got.Ports))
}
