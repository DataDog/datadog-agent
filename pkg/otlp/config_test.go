// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && test
// +build otlp,test

package otlp

import (
	"fmt"
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
		{path: "experimental/port/bindhost.yaml", enabled: true},
		{path: "experimental/port/disabled.yaml", enabled: false},
		{path: "experimental/port/invalid.yaml", enabled: true},
		{path: "experimental/port/nobindhost.yaml", enabled: true},

		{path: "experimental/receiver/noprotocols.yaml", enabled: true},
		{path: "experimental/receiver/portandreceiver.yaml", enabled: true},
		{path: "experimental/receiver/simple.yaml", enabled: true},
		{path: "experimental/receiver/advanced.yaml", enabled: true},

		{path: "stable/invalid_port_based.yaml", enabled: false},
		{path: "stable/receiver/noprotocols.yaml", enabled: true},
		{path: "stable/receiver/simple.yaml", enabled: true},
		{path: "stable/receiver/advanced.yaml", enabled: true},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig("./testdata/" + testInstance.path)
			require.NoError(t, err)
			assert.Equal(t, testInstance.enabled, IsEnabled(cfg))
		})
	}
}

func TestIsEnabledEnv(t *testing.T) {
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:9993")
	cfg, err := testutil.LoadConfig("./testdata/empty.yaml")
	require.NoError(t, err)
	assert.True(t, IsEnabled(cfg))
}

func TestFromAgentConfigReceiver(t *testing.T) {
	tests := []struct {
		path string
		cfg  PipelineConfig
		err  string
	}{
		{
			path: "experimental/port/bindhost.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 5678, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "experimental/port/nobindhost.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 5678, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "experimental/port/invalid.yaml",
			err:  fmt.Sprintf("internal trace port is invalid: -1 is out of [0, 65535] range"),
		},
		{
			path: "experimental/port/alldisabled.yaml",
			err:  "at least one OTLP signal needs to be enabled",
		},
		{
			path: "experimental/receiver/noprotocols.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "experimental/receiver/portandreceiver.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 5679, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "experimental/receiver/simple.yaml",
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
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "experimental/receiver/advanced.yaml",
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
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "stable/receiver/noprotocols.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "stable/receiver/simple.yaml",
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
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			path: "stable/receiver/advanced.yaml",
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
					"enabled":         true,
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

func TestFromEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		cfg  PipelineConfig
		err  string
	}{
		{
			name: "only gRPC",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "0.0.0.0:9999",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": map[string]interface{}{
							"endpoint": "0.0.0.0:9999",
						},
					},
				},
				MetricsEnabled: true,
				TracesEnabled:  true,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			name: "HTTP + gRPC",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "0.0.0.0:9997",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT": "0.0.0.0:9998",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": map[string]interface{}{
							"endpoint": "0.0.0.0:9997",
						},
						"http": map[string]interface{}{
							"endpoint": "0.0.0.0:9998",
						},
					},
				},
				MetricsEnabled: true,
				TracesEnabled:  true,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			name: "HTTP + gRPC, metrics config",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "0.0.0.0:9995",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT": "0.0.0.0:9996",
				"DD_OTLP_CONFIG_METRICS_DELTA_TTL":                "2400",
				"DD_OTLP_CONFIG_METRICS_HISTOGRAMS_MODE":          "counters",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": map[string]interface{}{
							"endpoint": "0.0.0.0:9995",
						},
						"http": map[string]interface{}{
							"endpoint": "0.0.0.0:9996",
						},
					},
				},
				MetricsEnabled: true,
				TracesEnabled:  true,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
					"delta_ttl":       "2400",
					"histograms": map[string]interface{}{
						"mode": "counters",
					},
				},
			},
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			for env, val := range testInstance.env {
				t.Setenv(env, val)
			}
			cfg, err := testutil.LoadConfig("./testdata/empty.yaml")
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
			path: "experimental/metrics/allconfig.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 5678, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"enabled":                     true,
					"delta_ttl":                   2400,
					"report_quantiles":            false,
					"send_monotonic_counter":      true,
					"resource_attributes_as_tags": true,
					"instrumentation_library_metadata_as_tags": true,
					"tag_cardinality":                          "orchestrator",
					"histograms": map[string]interface{}{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
			},
		},
		{
			path: "stable/metrics/allconfig.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 5678, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				TracesEnabled:      true,
				Metrics: map[string]interface{}{
					"enabled":                     true,
					"delta_ttl":                   2400,
					"report_quantiles":            false,
					"send_monotonic_counter":      true,
					"resource_attributes_as_tags": true,
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

func TestFromExperimentalEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		cfg  PipelineConfig
		err  string
	}{
		{
			name: "only gRPC",
			env: map[string]string{
				"DD_OTLP_GRPC_PORT": "4317",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": map[string]interface{}{
							"endpoint": "localhost:4317",
						},
					},
				},
				MetricsEnabled: true,
				TracesEnabled:  true,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			name: "only HTTP",
			env: map[string]string{
				"DD_OTLP_HTTP_PORT": "1234",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"http": map[string]interface{}{
							"endpoint": "localhost:1234",
						},
					},
				},
				MetricsEnabled: true,
				TracesEnabled:  true,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
		{
			name: "HTTP and gRPC",
			env: map[string]string{
				"DD_OTLP_GRPC_PORT": "4317",
				"DD_OTLP_HTTP_PORT": "1234",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": map[string]interface{}{
							"endpoint": "localhost:4317",
						},
						"http": map[string]interface{}{
							"endpoint": "localhost:1234",
						},
					},
				},
				MetricsEnabled: true,
				TracesEnabled:  true,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
			},
		},
	}
	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			for env, val := range testInstance.env {
				t.Setenv(env, val)
			}
			cfg, err := testutil.LoadConfig("./testdata/empty.yaml")
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
