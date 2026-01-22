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

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/configcheck"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
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
			cfg, err := testutil.LoadConfig(t, "./testdata/"+testInstance.path)
			require.NoError(t, err)
			assert.Equal(t, testInstance.enabled, configcheck.IsEnabled(cfg))
		})
	}
}

func TestIsEnabledEnv(t *testing.T) {
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:9993")
	cfg, err := testutil.LoadConfig(t, "./testdata/empty.yaml")
	require.NoError(t, err)
	assert.True(t, configcheck.IsEnabled(cfg))
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

				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
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

				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
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

				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
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
				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			path: "logs_enabled.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": true,
					"batch": map[string]interface{}{
						"min_size":      100,
						"max_size":      200,
						"flush_timeout": "5001ms",
					},
				},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			path: "logs_disabled.yaml",
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
				Debug: map[string]interface{}{},
			},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig(t, "./testdata/"+testInstance.path)
			require.NoError(t, err)
			pcfg, err := FromAgentConfig(cfg)
			if err != nil || testInstance.err != "" {
				assert.Equal(t, testInstance.err, err.Error())
				return
			}
			testInstance.cfg.Metrics, err = normalizeMetricsConfig(testInstance.cfg.Metrics, true)
			require.NoError(t, err)
			assert.Equal(t, testInstance.cfg, pcfg)
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

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
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

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
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
				"DD_OTLP_CONFIG_METRICS_BATCH_FLUSH_TIMEOUT":                    "5001ms",
				"DD_OTLP_CONFIG_METRICS_BATCH_MIN_SIZE":                         "100",
				"DD_OTLP_CONFIG_METRICS_BATCH_MAX_SIZE":                         "200",
				"DD_OTLP_CONFIG_METRICS_HISTOGRAMS_MODE":                        "counters",
				"DD_OTLP_CONFIG_METRICS_INSTRUMENTATION_SCOPE_METADATA_AS_TAGS": "false",
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

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"instrumentation_scope_metadata_as_tags": false,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",

					"delta_ttl": 2400,
					"histograms": map[string]interface{}{
						"mode": "counters",
					},
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "5001ms",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			name: "only gRPC, disabled logging",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "0.0.0.0:9999",
				"DD_OTLP_CONFIG_DEBUG_VERBOSITY":                  "none",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": map[string]interface{}{
							"endpoint": "0.0.0.0:9999",
						},
					},
				},

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
				Debug: map[string]interface{}{
					"verbosity": "none",
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

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
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

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			name: "logs enabled",
			env: map[string]string{
				"DD_OTLP_CONFIG_LOGS_ENABLED":             "true",
				"DD_OTLP_CONFIG_LOGS_BATCH_FLUSH_TIMEOUT": "5001ms",
				"DD_OTLP_CONFIG_LOGS_BATCH_MIN_SIZE":      "100",
				"DD_OTLP_CONFIG_LOGS_BATCH_MAX_SIZE":      "200",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": true,
					"batch": map[string]interface{}{
						"min_size":      100,
						"max_size":      200,
						"flush_timeout": "5001ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
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

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			name: "metrics resource_attributes_as_tags",
			env: map[string]string{
				"DD_OTLP_CONFIG_METRICS_RESOURCE_ATTRIBUTES_AS_TAGS": "true",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
				Debug: map[string]interface{}{},
			},
		},
		{
			name: "disable trace infra-attr processor",
			env: map[string]string{
				"DD_OTLP_CONFIG_TRACES_INFRA_ATTRIBUTES_ENABLED": "false",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: false,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"resource_attributes_as_tags":            false,
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
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
			cfg, err := testutil.LoadConfig(t, "./testdata/empty.yaml")
			require.NoError(t, err)
			pcfg, err := FromAgentConfig(cfg)
			if err != nil || testInstance.err != "" {
				assert.Equal(t, testInstance.err, err.Error())
				return
			}
			testInstance.cfg.Metrics, err = normalizeMetricsConfig(testInstance.cfg.Metrics, true)
			require.NoError(t, err)
			assert.Equal(t, testInstance.cfg, pcfg)
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

				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": true,
					"batch": map[string]interface{}{
						"min_size":      200,
						"max_size":      300,
						"flush_timeout": "4001ms",
					},
				},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"delta_ttl":                              2400,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": false,
					"tag_cardinality":                        "orchestrator",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"histograms": map[string]interface{}{
						"mode":                     "counters",
						"send_count_sum_metrics":   true,
						"send_aggregation_metrics": true,
					},
					"tags": "tag1:value1,tag2:value2",
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "5001ms",
				},
				Debug: map[string]interface{}{
					"verbosity": "detailed",
				},
			},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig(t, "./testdata/"+testInstance.path)
			require.NoError(t, err)
			pcfg, err := FromAgentConfig(cfg)
			if err != nil || testInstance.err != "" {
				assert.Equal(t, testInstance.err, err.Error())
				return
			}
			testInstance.cfg.Metrics, err = normalizeMetricsConfig(testInstance.cfg.Metrics, true)
			require.NoError(t, err)
			assert.Equal(t, testInstance.cfg, pcfg)
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
				OTLPReceiverConfig:           map[string]interface{}{},
				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				Debug: map[string]interface{}{},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
			},
		},
		{
			path:      "debug/verbosity_detailed.yaml",
			shouldSet: true,
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				Debug: map[string]interface{}{"verbosity": "detailed"},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
			},
		},
		{
			path:      "debug/verbosity_none.yaml",
			shouldSet: false,
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				Debug: map[string]interface{}{"verbosity": "none"},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
			},
		},
		{
			path:      "debug/verbosity_normal.yaml",
			shouldSet: true,
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"enabled": false,
					"batch": map[string]interface{}{
						"min_size":      8192,
						"max_size":      0,
						"flush_timeout": "200ms",
					},
				},
				Debug: map[string]interface{}{"verbosity": "normal"},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      8192,
					"max_size":      0,
					"flush_timeout": "200ms",
				},
			},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig(t, "./testdata/"+testInstance.path)
			require.NoError(t, err)
			pcfg, err := FromAgentConfig(cfg)
			if err != nil || testInstance.err != "" {
				assert.Equal(t, testInstance.err, err.Error())
				return
			}
			testInstance.cfg.Metrics, err = normalizeMetricsConfig(testInstance.cfg.Metrics, true)
			require.NoError(t, err)
			assert.Equal(t, testInstance.cfg, pcfg)
			assert.Equal(t, testInstance.shouldSet, pcfg.shouldSetLoggingSection())
		})
	}
}

func TestADPOTLPProxyOverridesEndpoints(t *testing.T) {
	env := map[string]string{
		"DD_DATA_PLANE_OTLP_PROXY_ENABLED":                          "true",
		"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":           "0.0.0.0:4317",
		"DD_DATA_PLANE_OTLP_PROXY_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "127.0.0.1:4319",
	}

	for k, v := range env {
		t.Setenv(k, v)
	}

	cfg, err := testutil.LoadConfig(t, "./testdata/empty.yaml")
	require.NoError(t, err)
	pcfg, err := FromAgentConfig(cfg)
	require.NoError(t, err)

	receiverConfig := pcfg.OTLPReceiverConfig
	protocols, _ := receiverConfig["protocols"].(map[string]interface{})

	grpc, _ := protocols["grpc"].(map[string]interface{})
	assert.Equal(t, "127.0.0.1:4319", grpc["endpoint"])
}

func TestADPOTLPProxyEmptyEndpointError(t *testing.T) {
	cfg, err := testutil.LoadConfig(t, "./testdata/adp_proxy_empty_grpc.yaml")
	require.NoError(t, err)
	_, err = FromAgentConfig(cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProxyGRPCEndpointNotConfigured)
}

func TestADPOTLPProxyEndpointCollisionError(t *testing.T) {
	t.Setenv("DD_DATA_PLANE_OTLP_PROXY_ENABLED", "true")
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:4317")
	t.Setenv("DD_DATA_PLANE_OTLP_PROXY_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:4317")

	cfg, err := testutil.LoadConfig(t, "./testdata/empty.yaml")
	require.NoError(t, err)
	_, err = FromAgentConfig(cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProxyGRPCEndpointCollision)
}
