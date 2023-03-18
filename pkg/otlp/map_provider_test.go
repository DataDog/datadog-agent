// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && test

package otlp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"

	"github.com/DataDog/datadog-agent/pkg/otlp/internal/testutil"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func TestNewMap(t *testing.T) {
	tests := []struct {
		name string
		pcfg PipelineConfig
		ocfg map[string]interface{}
	}{
		{
			name: "only gRPC, only Traces",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:          5003,
				TracesEnabled:      true,
				Debug: map[string]interface{}{
					"loglevel": "disabled",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"grpc": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
					},
				},
				"processors": map[string]interface{}{
					"batch": map[string]interface{}{
						"timeout": "10s",
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"otlp"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                                2000,
					"resource_attributes_as_tags":              true,
					"instrumentation_library_metadata_as_tags": true,
					"instrumentation_scope_metadata_as_tags":   true,
					"histograms": map[string]interface{}{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"loglevel": "disabled",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]interface{}{
					"batch": map[string]interface{}{
						"timeout": "10s",
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
					},
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                                2000,
							"resource_attributes_as_tags":              true,
							"instrumentation_library_metadata_as_tags": true,
							"instrumentation_scope_metadata_as_tags":   true,
							"histograms": map[string]interface{}{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"otlp"},
						},
						"metrics": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces, invalid loglevel(ignored)",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                                2000,
					"resource_attributes_as_tags":              true,
					"instrumentation_library_metadata_as_tags": true,
					"instrumentation_scope_metadata_as_tags":   true,
					"histograms": map[string]interface{}{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"loglevel": "foo",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]interface{}{
					"batch": map[string]interface{}{
						"timeout": "10s",
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
					},
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                                2000,
							"resource_attributes_as_tags":              true,
							"instrumentation_library_metadata_as_tags": true,
							"instrumentation_scope_metadata_as_tags":   true,
							"histograms": map[string]interface{}{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"otlp"},
						},
						"metrics": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "with both",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 5678),
				TracePort:          5003,
				TracesEnabled:      true,
				Debug: map[string]interface{}{
					"loglevel": "disabled",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"grpc": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
							"http": map[string]interface{}{
								"endpoint": "bindhost:5678",
							},
						},
					},
				},
				"processors": map[string]interface{}{
					"batch": map[string]interface{}{
						"timeout": "10s",
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"otlp"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, only metrics",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                                1500,
					"resource_attributes_as_tags":              false,
					"instrumentation_library_metadata_as_tags": false,
					"instrumentation_scope_metadata_as_tags":   false,
					"histograms": map[string]interface{}{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"loglevel": "disabled",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]interface{}{
					"batch": map[string]interface{}{
						"timeout": "10s",
					},
				},
				"exporters": map[string]interface{}{
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                                1500,
							"resource_attributes_as_tags":              false,
							"instrumentation_library_metadata_as_tags": false,
							"instrumentation_scope_metadata_as_tags":   false,
							"histograms": map[string]interface{}{
								"mode":                   "nobuckets",
								"send_count_sum_metrics": true,
							},
						},
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"metrics": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "only gRPC, only Traces, logging info",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:          5003,
				TracesEnabled:      true,
				Debug: map[string]interface{}{
					"loglevel": "info",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"grpc": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]interface{}{
					"batch": map[string]interface{}{
						"timeout": "10s",
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
					},
					"logging": map[string]interface{}{
						"loglevel": "info",
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"otlp", "logging"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, only metrics, logging debug",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                                1500,
					"resource_attributes_as_tags":              false,
					"instrumentation_library_metadata_as_tags": false,
					"histograms": map[string]interface{}{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"loglevel": "debug",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]interface{}{
					"batch": map[string]interface{}{
						"timeout": "10s",
					},
				},
				"exporters": map[string]interface{}{
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                                1500,
							"resource_attributes_as_tags":              false,
							"instrumentation_library_metadata_as_tags": false,
							"histograms": map[string]interface{}{
								"mode":                   "nobuckets",
								"send_count_sum_metrics": true,
							},
						},
					},
					"logging": map[string]interface{}{
						"loglevel": "debug",
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"metrics": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"serializer", "logging"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces, logging warn",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				Metrics: map[string]interface{}{
					"delta_ttl":                                2000,
					"resource_attributes_as_tags":              true,
					"instrumentation_library_metadata_as_tags": true,
					"histograms": map[string]interface{}{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				Debug: map[string]interface{}{
					"loglevel": "warn",
				},
			},
			ocfg: map[string]interface{}{
				"receivers": map[string]interface{}{
					"otlp": map[string]interface{}{
						"protocols": map[string]interface{}{
							"http": map[string]interface{}{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]interface{}{
					"batch": map[string]interface{}{
						"timeout": "10s",
					},
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
					},
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                                2000,
							"resource_attributes_as_tags":              true,
							"instrumentation_library_metadata_as_tags": true,
							"histograms": map[string]interface{}{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
					},
					"logging": map[string]interface{}{
						"loglevel": "warn",
					},
				},
				"service": map[string]interface{}{
					"telemetry": map[string]interface{}{"metrics": map[string]interface{}{"level": "none"}},
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"otlp", "logging"},
						},
						"metrics": map[string]interface{}{
							"receivers":  []interface{}{"otlp"},
							"processors": []interface{}{"batch"},
							"exporters":  []interface{}{"serializer", "logging"},
						},
					},
				},
			},
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			cfg, err := buildMap(testInstance.pcfg)
			require.NoError(t, err)
			tcfg := confmap.NewFromStringMap(testInstance.ocfg)
			assert.Equal(t, tcfg.ToStringMap(), cfg.ToStringMap())
		})
	}
}

func TestUnmarshal(t *testing.T) {
	provider, err := newMapProvider(PipelineConfig{
		OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 4317, 4318),
		TracePort:          5001,
		MetricsEnabled:     true,
		TracesEnabled:      true,
		Metrics: map[string]interface{}{
			"delta_ttl":                                2000,
			"resource_attributes_as_tags":              true,
			"instrumentation_library_metadata_as_tags": true,
			"instrumentation_scope_metadata_as_tags":   true,
			"histograms": map[string]interface{}{
				"mode":                   "counters",
				"send_count_sum_metrics": true,
			},
		},
	})
	require.NoError(t, err)
	components, err := getComponents(&serializer.MockSerializer{})
	require.NoError(t, err)

	_, err = provider.Get(context.Background(), components)
	require.NoError(t, err)
}
