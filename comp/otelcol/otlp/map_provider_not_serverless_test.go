// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && !serverless && test

package otlp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/logsagentexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/internal/configutils"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

func TestNewMap(t *testing.T) {
	tests := []struct {
		name string
		pcfg PipelineConfig
		ocfg map[string]any
	}{
		{
			name: "only gRPC, only Traces",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:                    5003,
				TracesEnabled:                true,
				TracesInfraAttributesEnabled: true,
				Debug: map[string]any{
					"verbosity": "none",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"grpc": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": nil,
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				TracesEnabled:                true,
				TracesInfraAttributesEnabled: true,
				MetricsEnabled:               true,
				Metrics: map[string]any{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]any{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "none",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": nil,
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                              2000,
							"resource_attributes_as_tags":            true,
							"instrumentation_scope_metadata_as_tags": true,
							"histograms": map[string]any{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp"},
						},
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces, invalid verbosity (ignored)",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				TracesEnabled:                true,
				MetricsEnabled:               true,
				TracesInfraAttributesEnabled: true,
				Metrics: map[string]any{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]any{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "foo",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": nil,
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                              2000,
							"resource_attributes_as_tags":            true,
							"instrumentation_scope_metadata_as_tags": true,
							"histograms": map[string]any{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp"},
						},
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "with both",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 1234, 5678),
				TracePort:                    5003,
				TracesEnabled:                true,
				TracesInfraAttributesEnabled: true,
				Debug: map[string]any{
					"verbosity": "none",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"grpc": map[string]any{
								"endpoint": "bindhost:1234",
							},
							"http": map[string]any{
								"endpoint": "bindhost:5678",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": nil,
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, only metrics",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesInfraAttributesEnabled: true,
				Metrics: map[string]any{
					"delta_ttl":                              1500,
					"resource_attributes_as_tags":            false,
					"instrumentation_scope_metadata_as_tags": false,
					"histograms": map[string]any{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "none",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": nil,
				},
				"exporters": map[string]any{
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                              1500,
							"resource_attributes_as_tags":            false,
							"instrumentation_scope_metadata_as_tags": false,
							"histograms": map[string]any{
								"mode":                   "nobuckets",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "only gRPC, only Traces, logging with normal verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:                    5003,
				TracesEnabled:                true,
				TracesInfraAttributesEnabled: true,
				Debug: map[string]any{
					"verbosity": "normal",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"grpc": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": nil,
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"debug": map[string]any{
						"verbosity": "normal",
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, only metrics, logging with detailed verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				MetricsEnabled:               true,
				TracesInfraAttributesEnabled: true,
				Metrics: map[string]any{
					"delta_ttl":                   1500,
					"resource_attributes_as_tags": false,
					"histograms": map[string]any{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "detailed",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": nil,
				},
				"exporters": map[string]any{
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                   1500,
							"resource_attributes_as_tags": false,
							"histograms": map[string]any{
								"mode":                   "nobuckets",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
					"debug": map[string]any{
						"verbosity": "detailed",
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces, logging with basic verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				TracesEnabled:                true,
				MetricsEnabled:               true,
				TracesInfraAttributesEnabled: true,
				Metrics: map[string]any{
					"delta_ttl":                   2000,
					"resource_attributes_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "basic",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": nil,
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                   2000,
							"resource_attributes_as_tags": true,
							"histograms": map[string]any{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
					"debug": map[string]any{
						"verbosity": "basic",
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp", "debug"},
						},
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only gRPC, traces and logs",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:                    5003,
				TracesEnabled:                true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Debug: map[string]any{
					"verbosity": "none",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"grpc": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"logsagent": map[string]any{
						"sending_queue": map[string]any{
							"batch": any(nil),
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP; metrics, logs and traces",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				TracesEnabled:                true,
				MetricsEnabled:               true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"batch": map[string]interface{}{
						"min_size":      100,
						"max_size":      200,
						"flush_timeout": "10s",
					},
				},
				Metrics: map[string]any{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "none",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                              2000,
							"resource_attributes_as_tags":            true,
							"instrumentation_scope_metadata_as_tags": true,
							"histograms": map[string]any{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
					"logsagent": map[string]any{
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp"},
						},
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP; metrics, logs and traces; invalid verbosity (ignored)",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				TracesEnabled:                true,
				MetricsEnabled:               true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"batch": map[string]interface{}{
						"min_size":      100,
						"max_size":      200,
						"flush_timeout": "10s",
					},
				},
				Metrics: map[string]any{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "foo",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                              2000,
							"resource_attributes_as_tags":            true,
							"instrumentation_scope_metadata_as_tags": true,
							"histograms": map[string]any{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
					"logsagent": map[string]any{
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp"},
						},
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "traces and logs, with both gRPC and HTTP",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 1234, 5678),
				TracePort:                    5003,
				TracesEnabled:                true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"batch": map[string]interface{}{
						"min_size":      100,
						"max_size":      200,
						"flush_timeout": "10s",
					},
				},
				Debug: map[string]any{
					"verbosity": "none",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"grpc": map[string]any{
								"endpoint": "bindhost:1234",
							},
							"http": map[string]any{
								"endpoint": "bindhost:5678",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"logsagent": map[string]any{
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and logs",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				MetricsEnabled:               true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"batch": map[string]interface{}{
						"min_size":      100,
						"max_size":      200,
						"flush_timeout": "10s",
					},
				},
				Metrics: map[string]any{
					"delta_ttl":                              1500,
					"resource_attributes_as_tags":            false,
					"instrumentation_scope_metadata_as_tags": false,
					"histograms": map[string]any{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "none",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
				},
				"exporters": map[string]any{
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                              1500,
							"resource_attributes_as_tags":            false,
							"instrumentation_scope_metadata_as_tags": false,
							"histograms": map[string]any{
								"mode":                   "nobuckets",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
					"logsagent": map[string]any{
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "only gRPC, traces and logs, logging with normal verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:                    5003,
				TracesEnabled:                true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"batch": map[string]interface{}{
						"min_size":      100,
						"max_size":      200,
						"flush_timeout": "10s",
					},
				},
				Debug: map[string]any{
					"verbosity": "normal",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"grpc": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"debug": map[string]any{
						"verbosity": "normal",
					},
					"logsagent": map[string]any{
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp", "debug"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"logsagent", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and logs, logging with detailed verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				MetricsEnabled:               true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"batch": map[string]interface{}{
						"min_size":      100,
						"max_size":      200,
						"flush_timeout": "10s",
					},
				},
				Metrics: map[string]any{
					"delta_ttl":                   1500,
					"resource_attributes_as_tags": false,
					"histograms": map[string]any{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "detailed",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
				},
				"exporters": map[string]any{
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                   1500,
							"resource_attributes_as_tags": false,
							"histograms": map[string]any{
								"mode":                   "nobuckets",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
					"debug": map[string]any{
						"verbosity": "detailed",
					},
					"logsagent": map[string]any{
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer", "debug"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"logsagent", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP; metrics, traces, and logs; logging with basic verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:                    5003,
				TracesEnabled:                true,
				MetricsEnabled:               true,
				LogsEnabled:                  true,
				TracesInfraAttributesEnabled: true,
				Logs: map[string]interface{}{
					"batch": map[string]interface{}{
						"min_size":      100,
						"max_size":      200,
						"flush_timeout": "10s",
					},
				},
				Metrics: map[string]any{
					"delta_ttl":                   2000,
					"resource_attributes_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
				},
				MetricsBatch: map[string]interface{}{
					"min_size":      100,
					"max_size":      200,
					"flush_timeout": "10s",
				},
				Debug: map[string]any{
					"verbosity": "basic",
				},
			},
			ocfg: map[string]any{
				"receivers": map[string]any{
					"otlp": map[string]any{
						"protocols": map[string]any{
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
				},
				"exporters": map[string]any{
					"otlp": map[string]any{
						"tls": map[string]any{
							"insecure": true,
						},
						"compression": "none",
						"endpoint":    "localhost:5003",
						"sending_queue": map[string]any{
							"enabled": false,
						},
					},
					"serializer": map[string]any{
						"metrics": map[string]any{
							"delta_ttl":                   2000,
							"resource_attributes_as_tags": true,
							"histograms": map[string]any{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
					"debug": map[string]any{
						"verbosity": "basic",
					},
					"logsagent": map[string]any{
						"sending_queue": map[string]any{
							"batch": map[string]any{
								"min_size":      100,
								"max_size":      200,
								"flush_timeout": "10s",
							},
						},
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"traces": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"otlp", "debug"},
						},
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"serializer", "debug"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes"},
							"exporters":  []any{"logsagent", "debug"},
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
	pcfg := PipelineConfig{
		OTLPReceiverConfig:           testutil.OTLPConfigFromPorts("localhost", 4317, 4318),
		TracePort:                    5001,
		MetricsEnabled:               true,
		TracesEnabled:                true,
		LogsEnabled:                  true,
		TracesInfraAttributesEnabled: true,
		Metrics: map[string]any{
			"delta_ttl":                              2000,
			"resource_attributes_as_tags":            true,
			"instrumentation_scope_metadata_as_tags": true,
			"histograms": map[string]any{
				"mode":                   "counters",
				"send_count_sum_metrics": true,
			},
		},
	}
	cfgMap, err := buildMap(pcfg)
	require.NoError(t, err)

	mapSettings := otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs: []string{"map:hardcoded"},
			ProviderFactories: []confmap.ProviderFactory{
				configutils.NewProviderFactory(cfgMap),
			},
		},
	}

	provider, err := otelcol.NewConfigProvider(mapSettings)
	require.NoError(t, err)
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	components, err := getComponents(serializermock.NewMetricSerializer(t), make(chan *message.Message), fakeTagger, hostnameimpl.NewHostnameService(), nil)
	require.NoError(t, err)

	svccfg, err := provider.Get(context.Background(), components)
	require.NoError(t, err)

	scfgRaw := svccfg.Exporters[component.MustNewID(serializerexporter.TypeStr)]
	require.NotNil(t, scfgRaw)
	scfg, ok := scfgRaw.(*serializerexporter.ExporterConfig)
	require.True(t, ok, "failed to cast serializerexporter.ExporterConfig")
	sBatchCfg := scfg.QueueBatchConfig.Get().Batch.Get()
	assert.Equal(t, 200*time.Millisecond, sBatchCfg.FlushTimeout)
	assert.Equal(t, int64(8192), sBatchCfg.MinSize)
	assert.Equal(t, int64(0), sBatchCfg.MaxSize)

	lcfgRaw := svccfg.Exporters[component.MustNewID(logsagentexporter.TypeStr)]
	require.NotNil(t, lcfgRaw)
	lcfg, ok := lcfgRaw.(*logsagentexporter.Config)
	require.True(t, ok, "failed to cast logsagentexporter.Config")
	lBatchCfg := lcfg.QueueSettings.Get().Batch.Get()
	assert.Equal(t, 200*time.Millisecond, lBatchCfg.FlushTimeout)
	assert.Equal(t, int64(8192), lBatchCfg.MinSize)
	assert.Equal(t, int64(0), lBatchCfg.MaxSize)
}
