// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package buildprofile

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/stretchr/testify/assert"
)

func Test_ShouldMergeMetadata(t *testing.T) {
	tests := []struct {
		name            string
		resourceConfigs []DefaultMetadataResourceConfig
		valueStore      *valuestore.ResultValueStore
		validConnection bool
		config          *checkconfig.CheckConfig
		expected        bool
	}{
		{
			name: "base metadata",
			resourceConfigs: []DefaultMetadataResourceConfig{
				DefaultMetadataConfigs[0]["device"],
				DefaultMetadataConfigs[0]["interface"],
				DefaultMetadataConfigs[0]["ip_addresses"],
			},
			valueStore:      buildEmptyValueStore(),
			validConnection: true,
			config:          &checkconfig.CheckConfig{},
			expected:        true,
		},
		{
			name: "topology metadata without CollectTopology",
			resourceConfigs: []DefaultMetadataResourceConfig{
				DefaultMetadataConfigs[1]["lldp_remote"],
				DefaultMetadataConfigs[1]["lldp_remote_management"],
				DefaultMetadataConfigs[1]["lldp_local"],
				DefaultMetadataConfigs[1]["cdp_remote"],
			},
			valueStore:      buildEmptyValueStore(),
			validConnection: true,
			config:          &checkconfig.CheckConfig{},
			expected:        false,
		},
		{
			name: "topology metadata with CollectTopology",
			resourceConfigs: []DefaultMetadataResourceConfig{
				DefaultMetadataConfigs[1]["lldp_remote"],
				DefaultMetadataConfigs[1]["lldp_remote_management"],
				DefaultMetadataConfigs[1]["lldp_local"],
				DefaultMetadataConfigs[1]["cdp_remote"],
			},
			valueStore:      buildEmptyValueStore(),
			validConnection: true,
			config: &checkconfig.CheckConfig{
				CollectTopology: true,
			},
			expected: true,
		},
		{
			name: "VPN tunnels, route table, and tunnel metadata without CollectVPN",
			resourceConfigs: []DefaultMetadataResourceConfig{
				DefaultMetadataConfigs[2]["cisco_ipsec_tunnel"],
				DefaultMetadataConfigs[3]["ipforward_deprecated"],
				DefaultMetadataConfigs[3]["ipforward"],
				DefaultMetadataConfigs[4]["tunnel_config_deprecated"],
				DefaultMetadataConfigs[4]["tunnel_config"],
			},
			valueStore:      buildEmptyValueStore(),
			validConnection: true,
			config:          &checkconfig.CheckConfig{},
			expected:        false,
		},
		{
			name: "VPN tunnels, route table, and tunnel metadata with CollectVPN and invalid session",
			resourceConfigs: []DefaultMetadataResourceConfig{
				DefaultMetadataConfigs[2]["cisco_ipsec_tunnel"],
				DefaultMetadataConfigs[3]["ipforward_deprecated"],
				DefaultMetadataConfigs[3]["ipforward"],
				DefaultMetadataConfigs[4]["tunnel_config_deprecated"],
				DefaultMetadataConfigs[4]["tunnel_config"],
			},
			valueStore:      buildEmptyValueStore(),
			validConnection: false,
			config: &checkconfig.CheckConfig{
				CollectVPN: true,
			},
			expected: true,
		},
		{
			name: "VPN tunnels, route table, and tunnel metadata with CollectVPN and OIDs are not present",
			resourceConfigs: []DefaultMetadataResourceConfig{
				DefaultMetadataConfigs[2]["cisco_ipsec_tunnel"],
				DefaultMetadataConfigs[3]["ipforward_deprecated"],
				DefaultMetadataConfigs[3]["ipforward"],
				DefaultMetadataConfigs[4]["tunnel_config_deprecated"],
				DefaultMetadataConfigs[4]["tunnel_config"],
			},
			valueStore:      buildEmptyValueStore(),
			validConnection: true,
			config: &checkconfig.CheckConfig{
				CollectVPN: true,
			},
			expected: false,
		},
		{
			name: "VPN tunnels, route table, and tunnel metadata with CollectVPN and OIDs are present",
			resourceConfigs: []DefaultMetadataResourceConfig{
				DefaultMetadataConfigs[2]["cisco_ipsec_tunnel"],
				DefaultMetadataConfigs[3]["ipforward_deprecated"],
				DefaultMetadataConfigs[3]["ipforward"],
				DefaultMetadataConfigs[4]["tunnel_config_deprecated"],
				DefaultMetadataConfigs[4]["tunnel_config"],
			},
			valueStore: &valuestore.ResultValueStore{
				ScalarValues: make(valuestore.ScalarResultValuesType),
				ColumnValues: valuestore.ColumnResultValuesType{
					"1.3.6.1.4.1.9.9.171.1.3.2.1.4": {
						"2": valuestore.ResultValue{
							Value: []byte{0x0A, 0x00, 0x00, 0x01},
						},
					},
					"1.3.6.1.2.1.4.24.4.1.5": {
						"100.1.0.0.255.255.0.0.0.0.0.0.0": valuestore.ResultValue{
							Value: 2,
						},
					},
					"1.3.6.1.2.1.4.24.7.1.7": {
						"2.16.255.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.2.0.0.0.0": valuestore.ResultValue{
							Value: 4,
						},
					},
					"1.3.6.1.2.1.10.131.1.1.2.1.5": {
						"10.0.0.1.20.0.0.1.1.1": valuestore.ResultValue{
							Value: 6,
						},
					},
					"1.3.6.1.2.1.10.131.1.1.3.1.6": {
						"1.4.10.0.2.91.4.34.230.217.35.1.1": valuestore.ResultValue{
							Value: 8,
						},
					},
				},
			},
			validConnection: true,
			config: &checkconfig.CheckConfig{
				CollectVPN: true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, resourceConfig := range tt.resourceConfigs {
				shouldMerge := resourceConfig.ShouldMergeMetadata(tt.valueStore, tt.validConnection, tt.config)
				assert.Equal(t, tt.expected, shouldMerge)
			}
		})
	}
}

func buildEmptyValueStore() *valuestore.ResultValueStore {
	return &valuestore.ResultValueStore{
		ScalarValues: make(valuestore.ScalarResultValuesType),
		ColumnValues: make(valuestore.ColumnResultValuesType),
	}
}
