// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && test

package otlp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/multierr"

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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
								"allowed_origins": []string{"http://test.com"},
								"allowed_headers": []string{"ExampleHeader"},
							},
						},
					},
				},
				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
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
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
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
							"max_recv_msg_size_mib": 10,
						},
					},
				},

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
			},
		},
		{
			name: "gRPC max_concurrent_streams via env",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":               "0.0.0.0:9999",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_MAX_CONCURRENT_STREAMS": "32",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{
					"protocols": map[string]interface{}{
						"grpc": map[string]interface{}{
							"endpoint":               "0.0.0.0:9999",
							"max_concurrent_streams": 32,
						},
					},
				},

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
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
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs: map[string]interface{}{
					"enabled": false,
				},
				TracePort: 5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"resource_attributes_as_tags":            false,
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
			},
		},
		{
			name: "metrics infra_attributes.as_tags",
			env: map[string]string{
				"DD_OTLP_CONFIG_METRICS_INFRA_ATTRIBUTES_AS_TAGS": "true",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				TracesContainerTagPromotion:  "off",
				MetricsInfraAttrsAsTags:      true,
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
			},
		},
		{
			// resource_attributes_as_tags already promotes every resource attribute
			// (including the custom tags infra_attributes.as_tags targets), so the two
			// are mutually exclusive: resource_attributes_as_tags wins and as_tags is
			// ignored to avoid leaking the internal datadog.container.tag. namespace.
			name: "metrics infra_attributes.as_tags ignored when resource_attributes_as_tags is set",
			env: map[string]string{
				"DD_OTLP_CONFIG_METRICS_INFRA_ATTRIBUTES_AS_TAGS":    "true",
				"DD_OTLP_CONFIG_METRICS_RESOURCE_ATTRIBUTES_AS_TAGS": "true",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				TracesContainerTagPromotion:  "off",
				MetricsInfraAttrsAsTags:      false,
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
			},
		},
		{
			name: "override trace container_tag_promotion",
			env: map[string]string{
				"DD_OTLP_CONFIG_TRACES_INFRA_ATTRIBUTES_CONTAINER_TAG_PROMOTION": "rename",
			},
			cfg: PipelineConfig{
				OTLPReceiverConfig: map[string]interface{}{},

				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				TracesContainerTagPromotion:  "rename",
				Logs:                         map[string]interface{}{},
				TracePort:                    5003,
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
				Debug:        map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
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
			// A fully absent debug section leaves the exporter detached: verbosity is
			// never populated, so no per-batch debug logs are emitted by default.
			path:      "debug/absent_debug.yaml",
			shouldSet: false,
			cfg: PipelineConfig{
				OTLPReceiverConfig:           map[string]interface{}{},
				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Debug:                        map[string]interface{}{},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
			},
		},
		{
			// Explicitly declaring the debug section without a verbosity attaches the
			// debug exporter using the default verbosity ("basic"). Only a fully absent
			// debug section or an explicit "none" leaves the exporter detached.
			path:      "debug/empty_but_set_debug.yaml",
			shouldSet: true,
			cfg: PipelineConfig{
				OTLPReceiverConfig:           map[string]interface{}{},
				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesEnabled:                true,
				LogsEnabled:                  false,
				TracesInfraAttributesEnabled: true,
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Debug:                        map[string]interface{}{"verbosity": "basic"},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Debug:                        map[string]interface{}{"verbosity": "detailed"},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Debug:                        map[string]interface{}{"verbosity": "none"},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
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
				TracesContainerTagPromotion:  "off",
				Logs:                         map[string]interface{}{},
				Debug:                        map[string]interface{}{"verbosity": "normal"},
				Metrics: map[string]interface{}{
					"enabled":                                true,
					"tag_cardinality":                        "low",
					"apm_stats_receiver_addr":                "http://localhost:8126/v0.6/stats",
					"instrumentation_scope_metadata_as_tags": true,
				},
				MetricsBatch: map[string]interface{}{},
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

func TestDataPlaneOTLPProxyRejectsReceiverTLS(t *testing.T) {
	paths := []string{
		"receiver/tls.yaml",
		"receiver/tls_null.yaml",
		"receiver/tls_empty.yaml",
		"receiver/tls_unix.yaml",
		"receiver/unix_grpc_tls_http.yaml",
	}
	for _, signal := range []string{"LOGS", "METRICS", "TRACES"} {
		for _, path := range paths {
			t.Run(signal+"/"+path, func(t *testing.T) {
				t.Setenv("DD_DATA_PLANE_ENABLED", "true")
				t.Setenv("DD_DATA_PLANE_OTLP_ENABLED", "true")
				t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_"+signal, "true")
				cfg, err := testutil.LoadConfig(t, "./testdata/"+path)
				require.NoError(t, err)
				before := cfg.Get("otlp_config.receiver")
				want := configcheck.ReadConfigSection(cfg, "otlp_config.receiver").ToStringMap()

				got, err := FromAgentConfig(cfg)
				require.ErrorIs(t, err, configcheck.ErrDataPlaneProxyTLS)
				// Relocation is not attempted, so Unix transports without a relocation endpoint report only TLS.
				assert.NotErrorIs(t, err, ErrAgentReceiverEndpointNotConfigured)
				assert.Equal(t, want, got.OTLPReceiverConfig)
				assert.Equal(t, before, cfg.Get("otlp_config.receiver"))
			})
		}
	}
}

func TestDataPlaneOnlySkipsRelocation(t *testing.T) {
	// adp_proxy_empty_grpc.yaml has empty relocation endpoints, which proxy mode rejects.
	for _, path := range []string{"receiver/tls.yaml", "receiver/advanced.yaml", "adp_proxy_empty_grpc.yaml"} {
		t.Run(path, func(t *testing.T) {
			t.Setenv("DD_DATA_PLANE_ENABLED", "true")
			t.Setenv("DD_DATA_PLANE_OTLP_ENABLED", "true")
			cfg, err := testutil.LoadConfig(t, "./testdata/"+path)
			require.NoError(t, err)
			before := cfg.Get("otlp_config.receiver")
			want := configcheck.ReadConfigSection(cfg, "otlp_config.receiver").ToStringMap()

			got, err := FromAgentConfig(cfg)
			require.NoError(t, err)
			assert.Equal(t, want, got.OTLPReceiverConfig)
			assert.Equal(t, before, cfg.Get("otlp_config.receiver"))
		})
	}
}

func TestDataPlaneOTLPKeepsSharedReceiverTLS(t *testing.T) {
	for _, proxy := range []string{"", "true"} {
		t.Run("proxy="+proxy, func(t *testing.T) {
			t.Setenv("DD_DATA_PLANE_ENABLED", "true")
			t.Setenv("DD_DATA_PLANE_OTLP_ENABLED", "true")
			if proxy != "" {
				t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_TRACES", proxy)
			}
			cfg, err := testutil.LoadConfig(t, "./testdata/receiver/tls.yaml")
			require.NoError(t, err)
			_, _ = FromAgentConfig(cfg)

			// The data plane reads the receiver TLS settings from the flattened configuration stream.
			settings, _ := cfg.AllFlattenedSettingsWithSequenceID()
			for key, value := range map[string]string{
				"otlp_config.receiver.protocols.grpc.tls.cert_file":      "/etc/datadog-agent/otlp/cert.pem",
				"otlp_config.receiver.protocols.grpc.tls.key_file":       "/etc/datadog-agent/otlp/key.pem",
				"otlp_config.receiver.protocols.grpc.tls.client_ca_file": "/etc/datadog-agent/otlp/ca.pem",
				"otlp_config.receiver.protocols.http.tls.cert_file":      "/etc/datadog-agent/otlp/cert.pem",
				"otlp_config.receiver.protocols.http.tls.key_file":       "/etc/datadog-agent/otlp/key.pem",
			} {
				assert.Equal(t, value, settings[key], key)
			}
		})
	}
}

func TestDataPlaneOTLPProxyFlagsRelocate(t *testing.T) {
	for _, signals := range [][]string{{"LOGS"}, {"METRICS"}, {"TRACES"}, {"LOGS", "METRICS", "TRACES"}} {
		t.Run(strings.Join(signals, ","), func(t *testing.T) {
			t.Setenv("DD_DATA_PLANE_ENABLED", "true")
			t.Setenv("DD_DATA_PLANE_OTLP_ENABLED", "true")
			for _, signal := range signals {
				t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_"+signal, "true")
			}
			cfg, err := testutil.LoadConfig(t, "./testdata/receiver/simple.yaml")
			require.NoError(t, err)

			got, err := FromAgentConfig(cfg)
			require.NoError(t, err)
			assert.Equal(t, map[string]interface{}{
				"protocols": map[string]interface{}{
					"grpc": map[string]interface{}{"endpoint": "127.0.0.1:4319"},
					"http": map[string]interface{}{"endpoint": "127.0.0.1:4320"},
				},
			}, got.OTLPReceiverConfig)
		})
	}
}

func TestDataPlaneOTLPRelocationGate(t *testing.T) {
	tests := []struct {
		name             string
		dataPlaneEnabled string
		otlpEnabled      string
	}{
		{name: "data plane disabled", dataPlaneEnabled: "false", otlpEnabled: "true"},
		{name: "OTLP disabled", dataPlaneEnabled: "true", otlpEnabled: "false"},
		{name: "both disabled", dataPlaneEnabled: "false", otlpEnabled: "false"},
		{name: "data plane unset", otlpEnabled: "true"},
		{name: "OTLP unset", dataPlaneEnabled: "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.dataPlaneEnabled != "" {
				t.Setenv("DD_DATA_PLANE_ENABLED", tt.dataPlaneEnabled)
			}
			if tt.otlpEnabled != "" {
				t.Setenv("DD_DATA_PLANE_OTLP_ENABLED", tt.otlpEnabled)
			}
			t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_AGENT_RECEIVER_GRPC_ENDPOINT", "0.0.0.0:5678")
			t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_AGENT_RECEIVER_HTTP_ENDPOINT", "localhost:1234")
			t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_LOGS", "true")
			t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_METRICS", "true")
			t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_TRACES", "true")
			// Receiver TLS, and relocation endpoints that collide with receiver/advanced.yaml, are ignored outside proxy mode.
			for _, path := range []string{"receiver/advanced.yaml", "receiver/tls.yaml"} {
				t.Run(path, func(t *testing.T) {
					cfg, err := testutil.LoadConfig(t, "./testdata/"+path)
					require.NoError(t, err)
					before := cfg.Get("otlp_config.receiver")
					want := configcheck.ReadConfigSection(cfg, "otlp_config.receiver").ToStringMap()

					got, err := FromAgentConfig(cfg)
					require.NoError(t, err)
					assert.Equal(t, want, got.OTLPReceiverConfig)
					assert.Equal(t, before, cfg.Get("otlp_config.receiver"))
				})
			}
		})
	}
}

func TestDataPlaneOTLPRelocatesReceiverListeners(t *testing.T) {
	const (
		grpcEndpointEnv = "DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_AGENT_RECEIVER_GRPC_ENDPOINT"
		httpEndpointEnv = "DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_AGENT_RECEIVER_HTTP_ENDPOINT"
	)
	defaultEndpoints := map[string]string{"grpc": "127.0.0.1:4319", "http": "127.0.0.1:4320"}
	tests := []struct {
		name        string
		path        string
		env         map[string]string
		endpoints   map[string]string
		err         error
		alsoErr     error
		errContains []string
		numErrs     int
	}{
		{
			name:      "both protocols with default relocation endpoints",
			path:      "receiver/advanced.yaml",
			endpoints: defaultEndpoints,
		},
		{
			name: "custom relocation endpoints from environment",
			path: "receiver/advanced.yaml",
			env: map[string]string{
				grpcEndpointEnv: "127.0.0.1:14319",
				httpEndpointEnv: "127.0.0.1:14320",
			},
			endpoints: map[string]string{"grpc": "127.0.0.1:14319", "http": "127.0.0.1:14320"},
		},
		{
			name: "gRPC only ignores configured HTTP relocation endpoint",
			path: "empty.yaml",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "0.0.0.0:4317",
				httpEndpointEnv: "127.0.0.1:4319",
			},
			endpoints: map[string]string{"grpc": "127.0.0.1:4319"},
		},
		{
			name: "HTTP only ignores configured gRPC relocation endpoint",
			path: "empty.yaml",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT": "0.0.0.0:4318",
				grpcEndpointEnv: "127.0.0.1:4320",
			},
			endpoints: map[string]string{"http": "127.0.0.1:4320"},
		},
		{
			name:      "null protocols are enabled",
			path:      "receiver/null.yaml",
			endpoints: defaultEndpoints,
		},
		{
			name:      "empty protocols are enabled",
			path:      "receiver/simple.yaml",
			endpoints: defaultEndpoints,
		},
		{
			name: "unix transport requires an explicit relocation endpoint",
			path: "empty.yaml",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":  "/tmp/configured-otlp.sock",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_TRANSPORT": "unix",
			},
			err:         ErrAgentReceiverEndpointNotConfigured,
			errContains: []string{"data_plane.experimental.otlp.proxy.agent_receiver_grpc_endpoint must be explicitly configured with a Unix socket path"},
		},
		{
			name: "unixgram transport requires an explicit relocation endpoint",
			path: "empty.yaml",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":  "/tmp/configured-otlp.sock",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_TRANSPORT": "unixgram",
			},
			err: ErrAgentReceiverEndpointNotConfigured,
		},
		{
			name: "unixpacket transport requires an explicit relocation endpoint",
			path: "empty.yaml",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":  "/tmp/configured-otlp.sock",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_TRANSPORT": "unixpacket",
			},
			err: ErrAgentReceiverEndpointNotConfigured,
		},
		{
			name: "preserves unix transport",
			path: "empty.yaml",
			env: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":  "/tmp/configured-otlp.sock",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_TRANSPORT": "unix",
				grpcEndpointEnv: "/tmp/agent-otlp.sock",
			},
			endpoints: map[string]string{"grpc": "/tmp/agent-otlp.sock"},
		},
		{
			name: "empty gRPC relocation endpoint",
			path: "adp_proxy_empty_grpc.yaml",
			err:  ErrAgentReceiverEndpointNotConfigured,
		},
		{
			name: "empty HTTP relocation endpoint",
			path: "adp_proxy_empty_grpc.yaml",
			env:  map[string]string{grpcEndpointEnv: "127.0.0.1:4319"},
			err:  ErrAgentReceiverEndpointNotConfigured,
		},
		{
			name: "explicit configured gRPC endpoint collision",
			path: "receiver/advanced.yaml",
			env:  map[string]string{grpcEndpointEnv: "0.0.0.0:5678"},
			err:  ErrAgentReceiverEndpointCollision,
		},
		{
			name: "explicit configured HTTP endpoint collision",
			path: "receiver/advanced.yaml",
			env:  map[string]string{httpEndpointEnv: "localhost:1234"},
			err:  ErrAgentReceiverEndpointCollision,
		},
		{
			name: "default configured gRPC endpoint collision",
			path: "receiver/simple.yaml",
			env:  map[string]string{grpcEndpointEnv: "localhost:4317"},
			err:  ErrAgentReceiverEndpointCollision,
		},
		{
			name: "default configured HTTP endpoint collision",
			path: "receiver/null.yaml",
			env:  map[string]string{httpEndpointEnv: "localhost:4318"},
			err:  ErrAgentReceiverEndpointCollision,
		},
		{
			name: "relocated gRPC collides with configured HTTP endpoint",
			path: "receiver/advanced.yaml",
			env:  map[string]string{grpcEndpointEnv: "localhost:1234"},
			err:  ErrAgentReceiverEndpointCollision,
		},
		{
			name: "relocated HTTP collides with configured gRPC endpoint",
			path: "receiver/advanced.yaml",
			env:  map[string]string{httpEndpointEnv: "0.0.0.0:5678"},
			err:  ErrAgentReceiverEndpointCollision,
		},
		{
			name: "swapping configured endpoints still collides",
			path: "receiver/advanced.yaml",
			env: map[string]string{
				grpcEndpointEnv: "localhost:1234",
				httpEndpointEnv: "0.0.0.0:5678",
			},
			err:     ErrAgentReceiverEndpointCollision,
			numErrs: 2,
		},
		{
			name:        "relocated endpoints collide",
			path:        "receiver/simple.yaml",
			env:         map[string]string{httpEndpointEnv: "127.0.0.1:4319"},
			err:         ErrAgentReceiverEndpointCollision,
			errContains: []string{"data_plane.experimental.otlp.proxy.agent_receiver_grpc_endpoint = \"127.0.0.1:4319\" conflicts with OTLP/http"},
			numErrs:     1,
		},
		{
			name: "endpoint comparison is exact",
			path: "receiver/simple.yaml",
			env: map[string]string{
				grpcEndpointEnv: "127.0.0.1:4317",
				httpEndpointEnv: "127.0.0.1:4318",
			},
			endpoints: map[string]string{"grpc": "127.0.0.1:4317", "http": "127.0.0.1:4318"},
		},
		{
			name:    "reports invalid gRPC relocation and HTTP collision together",
			path:    "adp_proxy_empty_grpc.yaml",
			env:     map[string]string{httpEndpointEnv: "0.0.0.0:4318"},
			err:     ErrAgentReceiverEndpointNotConfigured,
			alsoErr: ErrAgentReceiverEndpointCollision,
			errContains: []string{
				"data_plane.experimental.otlp.proxy.agent_receiver_grpc_endpoint",
				"data_plane.experimental.otlp.proxy.agent_receiver_http_endpoint = \"0.0.0.0:4318\" conflicts with OTLP/http",
			},
			numErrs: 2,
		},
		{
			name:      "wildcard relocation endpoint is allowed",
			path:      "receiver/advanced.yaml",
			env:       map[string]string{grpcEndpointEnv: "0.0.0.0:4319"},
			endpoints: map[string]string{"grpc": "0.0.0.0:4319", "http": "127.0.0.1:4320"},
		},
		{
			name: "no protocols ignores relocation endpoints",
			path: "receiver/noprotocols.yaml",
			env: map[string]string{
				grpcEndpointEnv: "localhost:4317",
				httpEndpointEnv: "localhost:4317",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_DATA_PLANE_ENABLED", "true")
			t.Setenv("DD_DATA_PLANE_OTLP_ENABLED", "true")
			t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_METRICS", "true")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			cfg, err := testutil.LoadConfig(t, "./testdata/"+tt.path)
			require.NoError(t, err)
			before := cfg.Get("otlp_config.receiver")
			want := configcheck.ReadConfigSection(cfg, "otlp_config.receiver").ToStringMap()
			for protocol, endpoint := range tt.endpoints {
				protocols, ok := want["protocols"].(map[string]interface{})
				require.True(t, ok)
				require.Contains(t, protocols, protocol)
				section, _ := protocols[protocol].(map[string]interface{})
				if section == nil {
					section = map[string]interface{}{}
				}
				section["endpoint"] = endpoint
				protocols[protocol] = section
			}

			got, err := FromAgentConfig(cfg)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				if tt.alsoErr != nil {
					require.ErrorIs(t, err, tt.alsoErr)
				}
				for _, contains := range tt.errContains {
					assert.ErrorContains(t, err, contains)
				}
				if tt.numErrs != 0 {
					assert.Len(t, multierr.Errors(err), tt.numErrs)
				}
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, want, got.OTLPReceiverConfig)
			assert.Equal(t, before, cfg.Get("otlp_config.receiver"))

			again, err := FromAgentConfig(cfg)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, got, again)
			assert.Equal(t, before, cfg.Get("otlp_config.receiver"))
		})
	}
}
