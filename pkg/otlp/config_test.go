// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test
// +build test

package otlp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/otlp/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		path    string
		enabled bool
	}{
		{path: "port/bindhost.yaml", enabled: true},
		{path: "port/disabled.yaml", enabled: false},
		{path: "port/invalid.yaml", enabled: true},
		{path: "port/nobindhost.yaml", enabled: true},
		{path: "port/nonlocal.yaml", enabled: true},

		{path: "receiver/noprotocols.yaml", enabled: true},
		{path: "receiver/portandreceiver.yaml", enabled: true},
		{path: "receiver/simple.yaml", enabled: true},
		{path: "receiver/advanced.yaml", enabled: true},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig("./testdata/" + testInstance.path)
			require.NoError(t, err)
			assert.Equal(t, testInstance.enabled, IsEnabled(cfg))
		})
	}
}

func TestFromAgentConfigReceiver(t *testing.T) {
	tests := []struct {
		path string
		cfg  PipelineConfig
		err  string
	}{
		{
			path: "port/bindhost.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 5678, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "port/nobindhost.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 5678, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "port/invalid.yaml",
			err: fmt.Sprintf("OTLP receiver port-based configuration is invalid: %s",
				strings.Join([]string{
					"HTTP port is invalid: -1 is out of [0, 65535] range",
					"gRPC port is invalid: -1 is out of [0, 65535] range",
					"internal trace port is invalid: -1 is out of [0, 65535] range",
				},
					"; ",
				),
			),
		},
		{
			path: "port/nonlocal.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("0.0.0.0", 5678, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "port/alldisabled.yaml",
			err:  "at least one OTLP signal needs to be enabled",
		},
		{
			path: "receiver/noprotocols.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "receiver/portandreceiver.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 5679, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "receiver/simple.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": nil,
						"http": nil,
					},
				},
				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				Metrics: map[string]interface{}{
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "receiver/advanced.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": map[string]interface{}{
							"endpoint":               "0.0.0.0:5678",
							"max_concurrent_streams": 16,
							"transport":              "tcp",
							"keepalive": map[string]interface{}{
								"enforcement_policy": map[string]interface{}{
									"min_time": "10m",
								},
							},
						},
						"http": map[string]interface{}{
							"endpoint": "localhost:1234",
							"cors": map[string]interface{}{
								"allowed_origins": []interface{}{"http://test.com"},
								"allowed_headers": []interface{}{"ExampleHeader"},
							},
						},
					},
				},
				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				Metrics: map[string]interface{}{
					"tag_cardinality": "low",
				},
			},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig("./testdata/" + testInstance.path)
			require.NoError(t, err)
			pcfg, err := FromAgentConfig(cfg)
			if err != nil || testInstance.err != "" {
				assert.Equal(t, testInstance.err, err.Error())
			} else {
				assert.Equal(t, testInstance.cfg, pcfg)
			}
		})
	}
}

func TestFromAgentConfigMetrics(t *testing.T) {
	tests := []struct {
		path string
		cfg  PipelineConfig
		err  string
	}{
		{
			path: "metrics/allconfig.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 5678, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"delta_ttl":                                2400,
					"report_quantiles":                         false,
					"send_monotonic_counter":                   true,
					"resource_attributes_as_tags":              true,
					"instrumentation_library_metadata_as_tags": true,
					"tag_cardinality":                          "orchestrator",
					"histograms": map[string]interface{}{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
			},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig("./testdata/" + testInstance.path)
			require.NoError(t, err)
			pcfg, err := FromAgentConfig(cfg)
			if err != nil || testInstance.err != "" {
				assert.Equal(t, testInstance.err, err.Error())
			} else {
				assert.Equal(t, testInstance.cfg, pcfg)
			}
		})
	}
}
