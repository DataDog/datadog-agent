// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/stretchr/testify/require"
)

func TestSyntheticsTestConfig_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		expectType  payload.Protocol
	}{
		{
			name: "UDP request",
			input: `{
				"version": 1,
				"type": "network",
				"subtype": "UDP",
				"config": {
					"assertions": [],
					"request": {
						"host": "example.com",
						"port": 53,
						"probe_count": 3
					}
				},
				"orgID": 42,
				"mainDC": "us-east-1",
				"publicID": "abc123"
			}`,
			expectError: false,
			expectType:  payload.ProtocolUDP,
		},
		{
			name: "TCP request",
			input: `{
				"version": 1,
				"type": "network",
				"subtype": "TCP",
				"config": {
					"assertions": [],
					"request": {
						"host": "example.com",
						"port": 80,
						"tcp_method": "SYN"
					}
				},
				"orgID": 42,
				"mainDC": "us-east-1",
				"publicID": "def456"
			}`,
			expectError: false,
			expectType:  payload.ProtocolTCP,
		},
		{
			name: "ICMP request",
			input: `{
				"version": 1,
				"type": "network",
				"subtype": "ICMP",
				"config": {
					"assertions": [],
					"request": {
						"host": "8.8.8.8"
					}
				},
				"orgID": 42,
				"mainDC": "us-east-1",
				"publicID": "ghi789"
			}`,
			expectError: false,
			expectType:  payload.ProtocolICMP,
		},
		{
			name: "Unknown subtype",
			input: `{
				"version": 1,
				"type": "network",
				"subtype": "foobar",
				"config": {
					"assertions": [],
					"request": {}
				},
				"orgID": 42,
				"mainDC": "us-east-1",
				"publicID": "xyz000"
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg SyntheticsTestConfig
			err := json.Unmarshal([]byte(tt.input), &cfg)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg.Config.Request)
			require.Equal(t, tt.expectType, cfg.Config.Request.GetSubType())
		})
	}
}

func TestSyntheticsTestConfig_UnmarshalJSON_AllFields(t *testing.T) {
	input := `{
		"version": 2,
		"type": "network",
		"subtype": "TCP",
		"config": {
			"assertions": [
				{
					"operator": "lessThan",
					"property": "avg",
					"target": "100",
					"type": "latency"
				}
			],
			"request": {
				"host": "example.com",
				"port": 443,
				"tcp_method": "syn",
				"source_service": "frontend",
				"destination_service": "backend",
				"probe_count": 5,
				"traceroute_count": 2,
				"max_ttl": 30,
				"timeout": 10
			}
		},
		"org_id": 101,
		"main_dc": "eu-west-1",
		"public_id": "pub-12345",
		"run_type": "on-demand",
		"tick_every": 60
	}`

	var actual SyntheticsTestConfig
	err := json.Unmarshal([]byte(input), &actual)
	require.NoError(t, err)

	port := 443
	probeCount := 5
	tracerouteCount := 2
	maxTTL := 30
	timeout := 10
	src := "frontend"
	dst := "backend"

	expected := SyntheticsTestConfig{
		Version:  2,
		Type:     "network",
		OrgID:    101,
		MainDC:   "eu-west-1",
		PublicID: "pub-12345",
		RunType:  "on-demand",
		Interval: 60,
		Config: struct {
			Assertions []Assertion   `json:"assertions"`
			Request    ConfigRequest `json:"request"`
		}{
			Assertions: []Assertion{
				{
					Operator: OperatorLessThan,
					Property: &[]AssertionSubType{AssertionSubTypeAverage}[0],
					Target:   "100",
					Type:     AssertionTypeLatency,
				},
			},
			Request: TCPConfigRequest{
				Host:      "example.com",
				Port:      &port,
				TCPMethod: payload.TCPMethod("syn"),
				NetworkConfigRequest: NetworkConfigRequest{
					SourceService:      &src,
					DestinationService: &dst,
					ProbeCount:         &probeCount,
					TracerouteCount:    &tracerouteCount,
					MaxTTL:             &maxTTL,
					Timeout:            &timeout,
				},
			},
		},
	}

	require.Equal(t, expected.Version, actual.Version)
	require.Equal(t, expected.Type, actual.Type)
	require.Equal(t, expected.OrgID, actual.OrgID)
	require.Equal(t, expected.MainDC, actual.MainDC)
	require.Equal(t, expected.PublicID, actual.PublicID)
	require.Equal(t, expected.RunType, actual.RunType)
	require.Equal(t, expected.Interval, actual.Interval)

	require.Equal(t, expected.Config.Assertions, actual.Config.Assertions)

	// Compare Request manually since it’s an interface
	actualReq, ok := actual.Config.Request.(TCPConfigRequest)
	require.True(t, ok)
	expectedReq := expected.Config.Request.(TCPConfigRequest)
	require.Equal(t, expectedReq, actualReq)
}
