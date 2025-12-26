// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package checkconfig

import (
	"maps"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/profile"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBuildProfile(t *testing.T) {
	metrics := []profiledefinition.MetricsConfig{
		{Symbol: profiledefinition.SymbolConfig{OID: "1.2.3.4.5", Name: "someMetric"}},
		{
			Symbols: []profiledefinition.SymbolConfig{
				{
					OID:  "1.2.3.4.6",
					Name: "abc",
				},
			},
			MetricTags: profiledefinition.MetricTagConfigList{
				profiledefinition.MetricTagConfig{
					Symbol: profiledefinition.SymbolConfigCompat{
						OID: "1.2.3.4.7",
					},
				},
			},
		},
	}
	profile1 := profiledefinition.ProfileDefinition{
		Name:    "profile1",
		Version: 12,
		Metrics: metrics,
		MetricTags: []profiledefinition.MetricTagConfig{
			{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
		},
		Metadata: profiledefinition.MetadataConfig{
			"device": {
				Fields: map[string]profiledefinition.MetadataField{
					"vendor": {
						Value: "a-vendor",
					},
					"description": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.1.99.3.0",
							Name: "sysDescr",
						},
					},
					"name": {
						Symbols: []profiledefinition.SymbolConfig{
							{
								OID:  "1.3.6.1.2.1.1.99.1.0",
								Name: "symbol1",
							},
							{
								OID:  "1.3.6.1.2.1.1.99.2.0",
								Name: "symbol2",
							},
						},
					},
				},
			},
			"interface": {
				Fields: map[string]profiledefinition.MetadataField{
					"oper_status": {
						Symbol: profiledefinition.SymbolConfig{
							OID:  "1.3.6.1.2.1.2.2.1.99",
							Name: "someIfSymbol",
						},
					},
				},
				IDTags: profiledefinition.MetricTagConfigList{
					{
						Tag: "interface",
						Symbol: profiledefinition.SymbolConfigCompat{
							OID:  "1.3.6.1.2.1.31.1.1.1.1",
							Name: "ifName",
						},
					},
				},
			},
		},
		SysObjectIDs: profiledefinition.StringArray{"1.1.1.*"},
	}

	mergedMetadata := make(profiledefinition.MetadataConfig)
	mergeMetadata(mergedMetadata, profile1.Metadata)
	mergedMetadata["ip_addresses"] = LegacyMetadataConfig["ip_addresses"]

	vpnTunnelsMergedMetadata := make(profiledefinition.MetadataConfig)
	maps.Copy(vpnTunnelsMergedMetadata, mergedMetadata)
	mergeMetadata(vpnTunnelsMergedMetadata, VPNTunnelMetadataConfig)
	mergeMetadata(vpnTunnelsMergedMetadata, RouteMetadataConfig)
	mergeMetadata(vpnTunnelsMergedMetadata, TunnelMetadataConfig)

	mockProfiles := profile.StaticProvider(profile.ProfileConfigMap{
		"profile1": profile.ProfileConfig{
			Definition: profile1,
		},
	})

	type testCase struct {
		name          string
		config        *CheckConfig
		sysObjectID   string
		expected      profiledefinition.ProfileDefinition
		expectedError string
	}
	for _, tc := range []testCase{
		{
			name: "inline",
			config: &CheckConfig{
				IPAddress:        "1.2.3.4",
				RequestedMetrics: metrics,
				RequestedMetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
				},
				ProfileName: ProfileNameInline,
			},
			expected: profiledefinition.ProfileDefinition{
				Metrics: metrics,
				MetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
				},
				Metadata: LegacyMetadataConfig,
			},
		},
		{
			name: "static",
			config: &CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     "profile1",
			},
			expected: profiledefinition.ProfileDefinition{
				Name:    "profile1",
				Version: 12,
				Metrics: metrics,
				MetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
				},
				StaticTags: []string{"snmp_profile:profile1"},
				Metadata:   mergedMetadata,
			},
		},
		{
			name: "dynamic",
			config: &CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     ProfileNameAuto,
			},
			sysObjectID: "1.1.1.1",
			expected: profiledefinition.ProfileDefinition{
				Name:    "profile1",
				Version: 12,
				Metrics: metrics,
				MetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
				},
				StaticTags: []string{"snmp_profile:profile1"},
				Metadata:   mergedMetadata,
			},
		},
		{
			name: "static with requested metrics",
			config: &CheckConfig{
				IPAddress:             "1.2.3.4",
				ProfileProvider:       mockProfiles,
				CollectDeviceMetadata: true,
				CollectTopology:       false,
				ProfileName:           "profile1",
				RequestedMetrics: []profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "3.1", Name: "global-metric"}}},
				RequestedMetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "global-tag", Symbol: profiledefinition.SymbolConfigCompat{OID: "3.2", Name: "globalSymbol"}},
				},
			},
			expected: profiledefinition.ProfileDefinition{
				Name:    "profile1",
				Version: 12,
				Metrics: append([]profiledefinition.MetricsConfig{
					{Symbol: profiledefinition.SymbolConfig{OID: "3.1", Name: "global-metric"}}},
					metrics...),
				MetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "global-tag", Symbol: profiledefinition.SymbolConfigCompat{OID: "3.2", Name: "globalSymbol"}},
					{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
				},
				Metadata:   mergedMetadata,
				StaticTags: []string{"snmp_profile:profile1"},
			},
		},
		{
			name: "static unknown",
			config: &CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     "f5",
			},
			expectedError: "unknown profile \"f5\"",
		},
		{
			name: "dynamic unknown",
			config: &CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     ProfileNameAuto,
			},
			sysObjectID: "3.3.3.3",
			expectedError: "failed to get profile for sysObjectID \"3.3.3.3\": no profiles found for sysObjectID \"3." +
				"3.3.3\"",
		},
		{
			name: "VPN tunnels metadata and metrics",
			config: &CheckConfig{
				IPAddress:       "1.2.3.4",
				ProfileProvider: mockProfiles,
				ProfileName:     "profile1",
				CollectVPN:      true,
			},
			expected: profiledefinition.ProfileDefinition{
				Name:    "profile1",
				Version: 12,
				Metrics: append(metrics, VPNTunnelMetrics...),
				MetricTags: []profiledefinition.MetricTagConfig{
					{Tag: "location", Symbol: profiledefinition.SymbolConfigCompat{OID: "1.3.6.1.2.1.1.6.0", Name: "sysLocation"}},
				},
				StaticTags: []string{"snmp_profile:profile1"},
				Metadata:   vpnTunnelsMergedMetadata,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile, err := tc.config.BuildProfile(tc.sysObjectID)
			if tc.expectedError != "" {
				assert.EqualError(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
				if !assert.Equal(t, tc.expected, profile) {
					for k, v := range tc.expected.Metadata["device"].Fields {
						t.Log(k, v)
					}
					t.Log("===")
					for k, v := range profile.Metadata["device"].Fields {
						t.Log(k, v)
					}
				}
			}
		})
	}
}
