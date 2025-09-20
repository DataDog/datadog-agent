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
