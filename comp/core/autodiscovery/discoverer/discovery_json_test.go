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
)

func TestParseDiscoveryResult(t *testing.T) {
	tests := []struct {
		name            string
		integration     string
		payload         string
		wantErr         bool
		wantNil         bool
		wantLen         int
		wantNames       []string
		wantInitConfigs []string // per-config; "" means default "{}"
		wantInstLens    []int
	}{
		{
			name:        "two configs use integration name",
			integration: "redis",
			payload: `[
				{"instances": [{"host": "10.0.0.1"}], "init_config": {"foo": 1}},
				{"instances": [{"host": "10.0.0.2"}]}
			]`,
			wantLen:         2,
			wantNames:       []string{"redis", "redis"},
			wantInitConfigs: []string{`{"foo":1}`, `{}`},
			wantInstLens:    []int{1, 1},
		},
		{
			name:            "integration name used when no name field",
			integration:     "krakend",
			payload:         `[{"instances":[{"host":"x"}]}]`,
			wantLen:         1,
			wantNames:       []string{"krakend"},
			wantInitConfigs: []string{`{}`},
			wantInstLens:    []int{1},
		},
		{
			name:        "empty array returns nil",
			integration: "redis",
			payload:     `[]`,
			wantNil:     true,
		},
		{
			name:        "invalid JSON returns error",
			integration: "redis",
			payload:     `not-json`,
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configs, err := parseDiscoveryResult(tc.integration, tc.payload)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				assert.Nil(t, configs)
				return
			}
			require.Len(t, configs, tc.wantLen)
			for i, cfg := range configs {
				assert.Equal(t, tc.wantNames[i], cfg.Name)
				assert.JSONEq(t, tc.wantInitConfigs[i], string(cfg.InitConfig))
				assert.Len(t, cfg.Instances, tc.wantInstLens[i])
			}
		})
	}
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
