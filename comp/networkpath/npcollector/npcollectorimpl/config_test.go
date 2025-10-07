// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/connfilter"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/pathteststore"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

func TestNetworkPathCollectorEnabled(t *testing.T) {
	config := &collectorConfigs{
		connectionsMonitoringEnabled: true,
	}
	assert.True(t, config.networkPathCollectorEnabled())

	config.connectionsMonitoringEnabled = false
	assert.False(t, config.networkPathCollectorEnabled())
}

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name           string
		configOverride map[string]any
		expectedConfig *collectorConfigs
	}{
		{
			name: "default configuration",
			configOverride: map[string]any{
				//"network_path.connections_monitoring.enabled": true,
				////"network_path.collector.workers":                        4,
				//"network_path.collector.timeout":                        3000,
				//"network_path.collector.max_ttl":                        30,
				//"network_path.collector.input_chan_size":                100,
				//"network_path.collector.processing_chan_size":           100,
				//"network_path.collector.pathtest_contexts_limit":        5000,
				//"network_path.collector.pathtest_ttl":                   90 * time.Second,
				//"network_path.collector.pathtest_interval":              60 * time.Second,
				//"network_path.collector.pathtest_max_per_minute":        100,
				//"network_path.collector.pathtest_max_burst_duration":    10 * time.Second,
				//"network_path.collector.flush_interval":                 60 * time.Second,
				//"network_path.collector.reverse_dns_enrichment.enabled": true,
				//"network_path.collector.reverse_dns_enrichment.timeout": 1000,
				//"network_path.collector.disable_intra_vpc_collection":   false,
				//"network_path.collector.tcp_method":                     "syn",
				//"network_path.collector.icmp_mode":                      "none",
				//"network_path.collector.tcp_syn_paris_traceroute_mode":  false,
				//"network_path.collector.traceroute_queries":             3,
				//"network_path.collector.e2e_queries":                    3,
				//"network_path.collector.disable_windows_driver":         false,
				//"network_path.collector.monitor_ip_without_domain":      false,
				//"network_devices.namespace":                             "default",
				//"site":                                                  "datadoghq.com",
			},
			expectedConfig: &collectorConfigs{
				connectionsMonitoringEnabled: false,
				workers:                      4,
				timeout:                      1000 * time.Millisecond,
				maxTTL:                       30,
				pathtestInputChanSize:        1000,
				pathtestProcessingChanSize:   1000,
				storeConfig: pathteststore.Config{
					ContextsLimit:    5000,
					TTL:              35 * time.Minute,
					Interval:         10 * time.Minute,
					MaxPerMinute:     150,
					MaxBurstDuration: 30 * time.Second,
				},
				flushInterval:             10 * time.Second,
				reverseDNSEnabled:         true,
				reverseDNSTimeout:         5000 * time.Millisecond,
				disableIntraVPCCollection: false,
				sourceExcludedConns:       map[string][]string{},
				destExcludedConns:         map[string][]string{},
				tcpMethod:                 "",
				icmpMode:                  "",
				tcpSynParisTracerouteMode: false,
				tracerouteQueries:         3,
				e2eQueries:                50,
				disableWindowsDriver:      false,
				networkDevicesNamespace:   "default",
				filterConfig:              nil,
				monitorIPWithoutDomain:    false,
				ddSite:                    "",
			},
		},
		{
			name: "custom configuration with filters",
			configOverride: map[string]any{
				"network_path.connections_monitoring.enabled":           false,
				"network_path.collector.workers":                        8,
				"network_path.collector.timeout":                        5000,
				"network_path.collector.max_ttl":                        64,
				"network_path.collector.input_chan_size":                200,
				"network_path.collector.processing_chan_size":           200,
				"network_path.collector.pathtest_contexts_limit":        10000,
				"network_path.collector.pathtest_ttl":                   120 * time.Second,
				"network_path.collector.pathtest_interval":              30 * time.Second,
				"network_path.collector.pathtest_max_per_minute":        200,
				"network_path.collector.pathtest_max_burst_duration":    20 * time.Second,
				"network_path.collector.flush_interval":                 30 * time.Second,
				"network_path.collector.reverse_dns_enrichment.enabled": false,
				"network_path.collector.reverse_dns_enrichment.timeout": 2000,
				"network_path.collector.disable_intra_vpc_collection":   true,
				"network_path.collector.tcp_method":                     "sack",
				"network_path.collector.icmp_mode":                      "all",
				"network_path.collector.tcp_syn_paris_traceroute_mode":  true,
				"network_path.collector.traceroute_queries":             5,
				"network_path.collector.e2e_queries":                    5,
				"network_path.collector.disable_windows_driver":         true,
				"network_path.collector.monitor_ip_without_domain":      true,
				"network_devices.namespace":                             "custom-ns",
				"site":                                                  "datadoghq.eu",
				"network_path.collector.source_excludes":                map[string][]string{"ip": {"192.168.1.1"}},
				"network_path.collector.dest_excludes":                  map[string][]string{"ip": {"10.0.0.1"}},
				"network_path.collector.filters": []map[string]any{
					{
						"type":         "include",
						"match_domain": "*.example.com",
						"match_ip":     "10.0.0.0/8",
					},
				},
			},
			expectedConfig: &collectorConfigs{
				connectionsMonitoringEnabled: false,
				workers:                      8,
				timeout:                      5000 * time.Millisecond,
				maxTTL:                       64,
				pathtestInputChanSize:        200,
				pathtestProcessingChanSize:   200,
				storeConfig: pathteststore.Config{
					ContextsLimit:    10000,
					TTL:              120 * time.Second,
					Interval:         30 * time.Second,
					MaxPerMinute:     200,
					MaxBurstDuration: 20 * time.Second,
				},
				flushInterval:             30 * time.Second,
				reverseDNSEnabled:         false,
				reverseDNSTimeout:         2000 * time.Millisecond,
				disableIntraVPCCollection: true,
				sourceExcludedConns:       map[string][]string{"ip": {"192.168.1.1"}},
				destExcludedConns:         map[string][]string{"ip": {"10.0.0.1"}},
				tcpMethod:                 payload.TCPConfigSACK,
				icmpMode:                  payload.ICMPModeAll,
				tcpSynParisTracerouteMode: true,
				tracerouteQueries:         5,
				e2eQueries:                5,
				disableWindowsDriver:      true,
				networkDevicesNamespace:   "custom-ns",
				filterConfig: []connfilter.Config{
					{
						Type:        "include",
						MatchDomain: "*.example.com",
						MatchIP:     "10.0.0.0/8",
					},
				},
				monitorIPWithoutDomain: true,
				ddSite:                 "datadoghq.eu",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := config.NewMockWithOverrides(t, tt.configOverride)
			mockLogger := logmock.New(t)

			result := newConfig(mockConfig, mockLogger)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedConfig.connectionsMonitoringEnabled, result.connectionsMonitoringEnabled)
			assert.Equal(t, tt.expectedConfig.workers, result.workers)
			assert.Equal(t, tt.expectedConfig.timeout, result.timeout)
			assert.Equal(t, tt.expectedConfig.maxTTL, result.maxTTL)
			assert.Equal(t, tt.expectedConfig.pathtestInputChanSize, result.pathtestInputChanSize)
			assert.Equal(t, tt.expectedConfig.pathtestProcessingChanSize, result.pathtestProcessingChanSize)
			assert.Equal(t, tt.expectedConfig.storeConfig, result.storeConfig)
			assert.Equal(t, tt.expectedConfig.flushInterval, result.flushInterval)
			assert.Equal(t, tt.expectedConfig.reverseDNSEnabled, result.reverseDNSEnabled)
			assert.Equal(t, tt.expectedConfig.reverseDNSTimeout, result.reverseDNSTimeout)
			assert.Equal(t, tt.expectedConfig.disableIntraVPCCollection, result.disableIntraVPCCollection)
			assert.Equal(t, tt.expectedConfig.sourceExcludedConns, result.sourceExcludedConns)
			assert.Equal(t, tt.expectedConfig.destExcludedConns, result.destExcludedConns)
			assert.Equal(t, tt.expectedConfig.tcpMethod, result.tcpMethod)
			assert.Equal(t, tt.expectedConfig.icmpMode, result.icmpMode)
			assert.Equal(t, tt.expectedConfig.tcpSynParisTracerouteMode, result.tcpSynParisTracerouteMode)
			assert.Equal(t, tt.expectedConfig.tracerouteQueries, result.tracerouteQueries)
			assert.Equal(t, tt.expectedConfig.e2eQueries, result.e2eQueries)
			assert.Equal(t, tt.expectedConfig.disableWindowsDriver, result.disableWindowsDriver)
			assert.Equal(t, tt.expectedConfig.networkDevicesNamespace, result.networkDevicesNamespace)
			assert.Equal(t, tt.expectedConfig.filterConfig, result.filterConfig)
			assert.Equal(t, tt.expectedConfig.monitorIPWithoutDomain, result.monitorIPWithoutDomain)
			assert.Equal(t, tt.expectedConfig.ddSite, result.ddSite)
		})
	}
}

func TestNewConfigInvalidFilters(t *testing.T) {
	// Test with invalid filter configuration that will cause unmarshalling error
	mockConfig := config.NewMockWithOverrides(t, map[string]any{
		"network_path.collector.filters": "invalid-string-should-be-array",
	})
	mockLogger := logmock.New(t)

	result := newConfig(mockConfig, mockLogger)

	// Should still return a config even with unmarshalling error
	require.NotNil(t, result)

	assert.Empty(t, result.filterConfig)
}
