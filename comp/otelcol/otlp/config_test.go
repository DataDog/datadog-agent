// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && test

package otlp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/internal/testutil"
)

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		path    string
		enabled bool
	}{
		{path: "invalid_port_based.yaml", enabled: false},
		{path: "receiver/noprotocols.yaml", enabled: true},
		{path: "receiver/simple.yaml", enabled: true},
		{path: "receiver/null.yaml", enabled: true},
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
			path: "receiver/noprotocols.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    false,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
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
				LogsEnabled:    false,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			path: "receiver/null.yaml",
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
				LogsEnabled:    false,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
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
							"max_recv_msg_size_mib": 10,
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
				LogsEnabled:    false,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			path: "logs_enabled.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    true,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			path: "logs_disabled.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    false,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
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
				LogsEnabled:    false,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
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
				LogsEnabled:    false,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			name: "HTTP + gRPC, metrics config",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":               "0.0.0.0:9995",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT":               "0.0.0.0:9996",
				"DD_OTLP_CONFIG_METRICS_DELTA_TTL":                              "2400",
				"DD_OTLP_CONFIG_METRICS_HISTOGRAMS_MODE":                        "counters",
				"DD_OTLP_CONFIG_METRICS_INSTRUMENTATION_SCOPE_METADATA_AS_TAGS": "true",
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
				LogsEnabled:    false,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"instrumentation_scope_metadata_as_tags": "true",
					"tag_cardinality":                        "low",
					"delta_ttl":                              "2400",
					"histograms": map[string]interface{}{
						"mode": "counters",
					},
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			name: "only gRPC, disabled logging",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "0.0.0.0:9999",
				"DD_OTLP_CONFIG_DEBUG_LOGLEVEL":                   "disabled",
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
				LogsEnabled:    false,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{
					"loglevel": "disabled",
				},
			},
		},
		{
			name: "only gRPC, verbosity normal",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "0.0.0.0:9999",
				"DD_OTLP_CONFIG_DEBUG_VERBOSITY":                  "normal",
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
				LogsEnabled:    false,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{
					"verbosity": "normal",
				},
			},
		},
		{
			name: "only gRPC, max receive message size 10",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":              "0.0.0.0:9999",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_MAX_RECV_MSG_SIZE_MIB": "10",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": map[string]interface{}{
							"endpoint":              "0.0.0.0:9999",
							"max_recv_msg_size_mib": "10",
						},
					},
				},

				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    false,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			name: "logs enabled",
			env: map[string]string{
				"DD_OTLP_CONFIG_LOGS_ENABLED": "true",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    true,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			name: "logs disabled",
			env: map[string]string{
				"DD_OTLP_CONFIG_LOGS_ENABLED": "false",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    false,
				TracePort:      5003,
				Metrics: map[string]interface{}{
					"enabled":         true,
					"tag_cardinality": "low",
				},
				Debug: map[string]interface{}{},
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
			path: "metrics/allconfig.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 5678, 1234),

				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    false,
				Metrics: map[string]interface{}{
					"enabled":                     true,
					"delta_ttl":                   2400,
					"resource_attributes_as_tags": true,
					"instrumentation_library_metadata_as_tags": true,
					"instrumentation_scope_metadata_as_tags":   true,
					"tag_cardinality":                          "orchestrator",
					"histograms": map[string]interface{}{
						"mode":                     "counters",
						"send_count_sum_metrics":   true,
						"send_aggregation_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"loglevel": "debug",
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

func TestFromAgentConfigDebug(t *testing.T) {
	tests := []struct {
		path      string
		cfg       PipelineConfig
		shouldSet bool
		err       string
	}{
		{
			path:      "debug/empty_but_set_debug.yaml",
			shouldSet: true,
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    false,
				Debug:          map[string]interface{}{},
				Metrics:        map[string]interface{}{"enabled": true, "tag_cardinality": "low"},
			},
		},
		{
			path:      "debug/loglevel_debug.yaml",
			shouldSet: true,
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    false,
				Debug:          map[string]interface{}{"loglevel": "debug"},
				Metrics:        map[string]interface{}{"enabled": true, "tag_cardinality": "low"},
			},
		},
		{
			path:      "debug/loglevel_disabled.yaml",
			shouldSet: false,
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    false,
				Debug:          map[string]interface{}{"loglevel": "disabled"},
				Metrics:        map[string]interface{}{"enabled": true, "tag_cardinality": "low"},
			},
		},
		{
			path:      "debug/verbosity_normal.yaml",
			shouldSet: true,
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:      5003,
				MetricsEnabled: true,
				TracesEnabled:  true,
				LogsEnabled:    false,
				Debug:          map[string]interface{}{"verbosity": "normal"},
				Metrics:        map[string]interface{}{"enabled": true, "tag_cardinality": "low"},
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
				assert.Equal(t, testInstance.shouldSet, pcfg.shouldSetLoggingSection())
			}
		})
	}
}
