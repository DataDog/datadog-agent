// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
)

func TestReadConfig(t *testing.T) {
	var tests = []struct {
		name           string
		configYaml     string
		expectedConfig NetflowConfig
		expectedError  string
	}{
		{
			name: "basic configs",
			configYaml: `
network_devices:
  netflow:
    enabled: true
    stop_timeout: 10
    aggregator_buffer_size: 20
    aggregator_flush_interval: 30
    aggregator_flow_context_ttl: 40
    aggregator_port_rollup_threshold: 20
    aggregator_rollup_tracker_refresh_interval: 60
    log_payloads: true
    aggregator_port_rollup_disabled: true
    prometheus_listener_enabled: true
    prometheus_listener_address: 127.0.0.1:9099
    listeners:
      - flow_type: netflow9
        bind_host: 127.0.0.1
        port: 1234
        workers: 10
        namespace: my-ns1
      - flow_type: netflow5
        bind_host: 127.0.0.2
        port: 2222
        workers: 15
        namespace: |
          my-ns2<abc
          zz
`,
			expectedConfig: NetflowConfig{
				Enabled:                                true,
				StopTimeout:                            10,
				AggregatorBufferSize:                   20,
				AggregatorFlushInterval:                30,
				AggregatorFlowContextTTL:               40,
				AggregatorPortRollupThreshold:          20,
				AggregatorRollupTrackerRefreshInterval: 60,
				AggregatorPortRollupDisabled:           true,
				PrometheusListenerEnabled:              true,
				PrometheusListenerAddress:              "127.0.0.1:9099",
				Listeners: []ListenerConfig{
					{
						FlowType:  common.TypeNetFlow9,
						BindHost:  "127.0.0.1",
						Port:      uint16(1234),
						Workers:   10,
						Namespace: "my-ns1",
					},
					{
						FlowType:  common.TypeNetFlow5,
						BindHost:  "127.0.0.2",
						Port:      uint16(2222),
						Workers:   15,
						Namespace: "my-ns2-abczz",
					},
				},
			},
		},
		{
			name: "defaults",
			configYaml: `
network_devices:
  netflow:
    enabled: true
    listeners:
      - flow_type: netflow9
`,
			expectedConfig: NetflowConfig{
				Enabled:                                true,
				StopTimeout:                            5,
				AggregatorBufferSize:                   10000,
				AggregatorFlushInterval:                300,
				AggregatorFlowContextTTL:               300,
				AggregatorPortRollupThreshold:          10,
				AggregatorRollupTrackerRefreshInterval: 300,
				PrometheusListenerAddress:              "localhost:9090",
				Listeners: []ListenerConfig{
					{
						FlowType:  common.TypeNetFlow9,
						BindHost:  "0.0.0.0",
						Port:      uint16(2055),
						Workers:   1,
						Namespace: "default",
					},
				},
			},
		},
		{
			name: "flow context ttl equal to flush interval if not defined",
			configYaml: `
network_devices:
  netflow:
    enabled: true
    aggregator_flush_interval: 50
    listeners:
      - flow_type: netflow9
`,
			expectedConfig: NetflowConfig{
				Enabled:                                true,
				StopTimeout:                            5,
				AggregatorBufferSize:                   10000,
				AggregatorFlushInterval:                50,
				AggregatorFlowContextTTL:               50,
				AggregatorPortRollupThreshold:          10,
				AggregatorRollupTrackerRefreshInterval: 300,
				PrometheusListenerAddress:              "localhost:9090",
				Listeners: []ListenerConfig{
					{
						FlowType:  common.TypeNetFlow9,
						BindHost:  "0.0.0.0",
						Port:      uint16(2055),
						Workers:   1,
						Namespace: "default",
					},
				},
			},
		},
		{
			name: "invalid flow type",
			configYaml: `
network_devices:
  netflow:
    enabled: true
    listeners:
      - flow_type: invalidType
`,
			expectedError: "the provided flow type `invalidType` is not valid",
		},
		{
			name: "invalid namespace with >100 chars",
			configYaml: `
network_devices:
  namespace: abcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefg
  netflow:
    enabled: true
    listeners:
      - flow_type: netflow9
`,
			expectedError: "invalid namespace `abcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefgabcdefg` error: namespace is too long, should contain less than 100 characters",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.Datadog.SetConfigType("yaml")
			err := config.Datadog.ReadConfig(strings.NewReader(tt.configYaml))
			require.NoError(t, err)

			readConfig, err := ReadConfig(config.Datadog)
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
				assert.Nil(t, readConfig)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedConfig, *readConfig)
			}
		})
	}
}

func TestListenerConfig_Addr(t *testing.T) {
	listenerConfig := ListenerConfig{
		FlowType: common.TypeNetFlow9,
		BindHost: "127.0.0.1",
		Port:     1234,
	}
	assert.Equal(t, "127.0.0.1:1234", listenerConfig.Addr())
}
