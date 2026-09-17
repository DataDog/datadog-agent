// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/connfilter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/pathteststore"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
)

func TestPathtestIntervalYAML(t *testing.T) {
	tests := []struct {
		name     string
		settings string
		want     time.Duration
	}{
		{name: "default", want: 30 * time.Minute},
		{name: "legacy duration", settings: `pathtest_interval: "45s"`, want: 45 * time.Second},
		{name: "legacy number remains nanoseconds", settings: `pathtest_interval: 45`, want: 45 * time.Nanosecond},
		{name: "legacy numeric string remains nanoseconds", settings: `pathtest_interval: "45"`, want: 45 * time.Nanosecond},
		{name: "seconds", settings: `pathtest_interval_sec: 30`, want: 30 * time.Second},
		{name: "quoted seconds", settings: `pathtest_interval_sec: "30"`, want: 30 * time.Second},
		// Native YAML numbers follow the shared config loader's integer coercion.
		{name: "YAML fraction is truncated", settings: `pathtest_interval_sec: 1.5`, want: time.Second},
		{name: "quoted fraction is rejected", settings: `pathtest_interval_sec: "1.5"`, want: 30 * time.Minute},
		{name: "minimum", settings: `pathtest_interval_sec: 1`, want: time.Second},
		{name: "maximum", settings: `pathtest_interval_sec: 9223372036`, want: 9223372036 * time.Second},
		{
			name:     "seconds take precedence",
			settings: "pathtest_interval: 45s\n    pathtest_interval_sec: 30",
			want:     30 * time.Second,
		},
		{
			name:     "explicit default seconds take precedence",
			settings: "pathtest_interval: 45s\n    pathtest_interval_sec: 1800",
			want:     30 * time.Minute,
		},
		{
			name:     "invalid seconds fall back to custom legacy interval",
			settings: "pathtest_interval: 45s\n    pathtest_interval_sec: 0",
			want:     45 * time.Second,
		},
		{name: "zero", settings: `pathtest_interval_sec: 0`, want: 30 * time.Minute},
		{name: "negative", settings: `pathtest_interval_sec: -1`, want: 30 * time.Minute},
		{name: "suffix rejected", settings: `pathtest_interval_sec: "30s"`, want: 30 * time.Minute},
		{name: "malformed", settings: `pathtest_interval_sec: "invalid"`, want: 30 * time.Minute},
		{name: "duration overflow", settings: `pathtest_interval_sec: 9223372037`, want: 30 * time.Minute},
		{name: "integer overflow", settings: `pathtest_interval_sec: "9223372036854775808"`, want: 30 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := config.NewMockFromYAML(t, "network_path:\n  collector:\n    "+tt.settings)
			result := newConfig(mockConfig, logmock.New(t))
			assert.Equal(t, tt.want, result.storeConfig.Interval)
		})
	}
}

func TestPathtestIntervalEnv(t *testing.T) {
	tests := []struct {
		name    string
		seconds string
		want    time.Duration
	}{
		{name: "seconds", seconds: "30", want: 30 * time.Second},
		{name: "explicit default", seconds: "1800", want: 30 * time.Minute},
		{name: "zero", seconds: "0", want: 45 * time.Second},
		{name: "negative", seconds: "-1", want: 45 * time.Second},
		{name: "fractional", seconds: "1.5", want: 45 * time.Second},
		{name: "suffix rejected", seconds: "30s", want: 45 * time.Second},
		{name: "duration overflow", seconds: "9223372037", want: 45 * time.Second},
		{name: "integer overflow", seconds: "9223372036854775808", want: 45 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_NETWORK_PATH_COLLECTOR_PATHTEST_INTERVAL_SEC", tt.seconds)
			mockConfig := config.NewMockFromYAML(t, `
network_path:
  collector:
    pathtest_interval: 45s
    pathtest_interval_sec: 60
`)
			result := newConfig(mockConfig, logmock.New(t))
			assert.Equal(t, tt.want, result.storeConfig.Interval)
		})
	}

	for _, tt := range []struct {
		value string
		want  time.Duration
	}{
		{value: "45s", want: 45 * time.Second},
		{value: "45", want: 45 * time.Nanosecond},
	} {
		t.Run("legacy env "+tt.value, func(t *testing.T) {
			t.Setenv("DD_NETWORK_PATH_COLLECTOR_PATHTEST_INTERVAL", tt.value)
			mockConfig := config.NewMock(t)
			result := newConfig(mockConfig, logmock.New(t))
			assert.Equal(t, tt.want, result.storeConfig.Interval)
		})
		t.Run("seconds override legacy env "+tt.value, func(t *testing.T) {
			t.Setenv("DD_NETWORK_PATH_COLLECTOR_PATHTEST_INTERVAL", tt.value)
			mockConfig := config.NewMockFromYAML(t, "network_path:\n  collector:\n    pathtest_interval_sec: 30")
			result := newConfig(mockConfig, logmock.New(t))
			assert.Equal(t, 30*time.Second, result.storeConfig.Interval)
		})
	}
}

func TestPathtestIntervalScheduling(t *testing.T) {
	mockConfig := config.NewMockFromYAML(t, `
network_path:
  collector:
    pathtest_interval: 1m
    pathtest_interval_sec: 30
`)
	logger := logmock.New(t)
	cfg := newConfig(mockConfig, logger)
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	now := start
	store := pathteststore.NewPathtestStore(cfg.storeConfig, logger, &teststatsd.Client{}, func() time.Time { return now })
	store.Add(&common.Pathtest{Hostname: "192.0.2.1"})

	// Advance the injected clock to prove that the configured seconds control
	// repeated scheduling, including the boundary just before each run is due.
	for _, step := range []struct {
		elapsed time.Duration
		want    int
	}{
		{0, 1},
		{29 * time.Second, 0},
		{30 * time.Second, 1},
		{59 * time.Second, 0},
		{60 * time.Second, 1},
	} {
		now = start.Add(step.elapsed)
		assert.Len(t, store.Flush(), step.want, "elapsed: %s", step.elapsed)
	}
}

func TestNetworkPathCollectorEnabled(t *testing.T) {
	config := &collectorConfigs{
		connectionsMonitoringEnabled: true,
	}
	assert.True(t, config.networkPathCollectorEnabled())

	config.connectionsMonitoringEnabled = false
	assert.False(t, config.networkPathCollectorEnabled())

	config.basicTestsEnabled = true
	assert.True(t, config.networkPathCollectorEnabled())

	config.basicTestsEnabled = false
	config.netflowMonitoringEnabled = true
	assert.True(t, config.networkPathCollectorEnabled())
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
				"network_path.collector.filters": []map[string]any{},
			},
			expectedConfig: &collectorConfigs{
				connectionsMonitoringEnabled: false,
				basicTestsEnabled:            false,
				netflowMonitoringEnabled:     false,
				workers:                      4,
				timeout:                      1000 * time.Millisecond,
				maxTTL:                       30,
				pathtestInputChanSize:        1000,
				pathtestProcessingChanSize:   1000,
				storeConfig: pathteststore.Config{
					ContextsLimit:    1000,
					TTL:              70 * time.Minute,
					Interval:         30 * time.Minute,
					MaxPerMinute:     150,
					MaxBurstDuration: 30 * time.Second,
				},
				flushInterval:                   10 * time.Second,
				reverseDNSEnabled:               true,
				reverseDNSTimeout:               5000 * time.Millisecond,
				disableIntraVPCCollection:       false,
				sourceExcludedConns:             map[string][]string{},
				destExcludedConns:               map[string][]string{},
				tcpMethod:                       "",
				icmpMode:                        "",
				tcpSynParisTracerouteMode:       false,
				tracerouteQueries:               3,
				e2eQueries:                      50,
				disableWindowsDriver:            false,
				disableSourcePublicIPCollection: false,
				networkDevicesNamespace:         "default",
				filterConfig:                    []connfilter.Config{},
				monitorIPWithoutDomain:          false,
				ddSite:                          "datadoghq.com",
				sourceProduct:                   payload.SourceProductNetworkPath,
			},
		},
		{
			name: "custom configuration with filters",
			configOverride: map[string]any{
				"network_path.connections_monitoring.enabled":                false,
				"network_path.collector.workers":                             8,
				"network_path.collector.timeout":                             5000,
				"network_path.collector.max_ttl":                             64,
				"network_path.collector.input_chan_size":                     200,
				"network_path.collector.processing_chan_size":                200,
				"network_path.collector.pathtest_contexts_limit":             10000,
				"network_path.collector.pathtest_ttl":                        120 * time.Second,
				"network_path.collector.pathtest_interval":                   30 * time.Second,
				"network_path.collector.pathtest_max_per_minute":             200,
				"network_path.collector.pathtest_max_burst_duration":         20 * time.Second,
				"network_path.collector.flush_interval":                      30 * time.Second,
				"network_path.collector.reverse_dns_enrichment.enabled":      false,
				"network_path.collector.reverse_dns_enrichment.timeout":      2000,
				"network_path.collector.disable_intra_vpc_collection":        true,
				"network_path.collector.tcp_method":                          "sack",
				"network_path.collector.icmp_mode":                           "all",
				"network_path.collector.tcp_syn_paris_traceroute_mode":       true,
				"network_path.collector.traceroute_queries":                  5,
				"network_path.collector.e2e_queries":                         5,
				"network_path.collector.disable_windows_driver":              true,
				"network_path.collector.disable_source_public_ip_collection": true,
				"network_path.collector.monitor_ip_without_domain":           true,
				"network_devices.namespace":                                  "custom-ns",
				"site":                                                       "datadoghq.eu",
				"network_path.collector.source_excludes":                     map[string][]string{"ip": {"192.168.1.1"}},
				"network_path.collector.dest_excludes":                       map[string][]string{"ip": {"10.0.0.1"}},
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
				basicTestsEnabled:            false,
				netflowMonitoringEnabled:     false,
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
				flushInterval:                   30 * time.Second,
				reverseDNSEnabled:               false,
				reverseDNSTimeout:               2000 * time.Millisecond,
				disableIntraVPCCollection:       true,
				sourceExcludedConns:             map[string][]string{"ip": {"192.168.1.1"}},
				destExcludedConns:               map[string][]string{"ip": {"10.0.0.1"}},
				tcpMethod:                       payload.TCPConfigSACK,
				icmpMode:                        payload.ICMPModeAll,
				tcpSynParisTracerouteMode:       true,
				tracerouteQueries:               5,
				e2eQueries:                      5,
				disableWindowsDriver:            true,
				disableSourcePublicIPCollection: true,
				networkDevicesNamespace:         "custom-ns",
				filterConfig: []connfilter.Config{
					{
						Type:        "include",
						MatchDomain: "*.example.com",
						MatchIP:     "10.0.0.0/8",
					},
				},
				monitorIPWithoutDomain: true,
				ddSite:                 "datadoghq.eu",
				sourceProduct:          payload.SourceProductNetworkPath,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := config.NewMockWithOverrides(t, tt.configOverride)
			mockLogger := logmock.New(t)

			result := newConfig(mockConfig, mockLogger)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedConfig, result)
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

func TestNewConfigFiltersFromEnv(t *testing.T) {
	t.Setenv("DD_NETWORK_PATH_COLLECTOR_FILTERS", `[
		{"match_domain":"*.example.com","type":"exclude"},
		{"match_domain":"^api-[0-9]+\\.example\\.com$","match_domain_strategy":"regex","type":"include"},
		{"match_ip":"10.0.0.0/8","type":"exclude"}
	]`)

	mockConfig := config.NewMock(t)
	result := newConfig(mockConfig, logmock.New(t))

	require.Equal(t, []connfilter.Config{
		{
			Type:        connfilter.FilterTypeExclude,
			MatchDomain: "*.example.com",
		},
		{
			Type:                connfilter.FilterTypeInclude,
			MatchDomain:         `^api-[0-9]+\.example\.com$`,
			MatchDomainStrategy: connfilter.MatchDomainStrategyRegex,
		},
		{
			Type:    connfilter.FilterTypeExclude,
			MatchIP: "10.0.0.0/8",
		},
	}, result.filterConfig)
}
