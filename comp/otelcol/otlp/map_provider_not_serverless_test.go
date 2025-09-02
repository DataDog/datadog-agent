// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && !serverless && test
// +build otlp,!serverless,test

package otlp

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
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
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:          5003,
				TracesEnabled:      true,
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
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				Metrics: map[string]any{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
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
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"batch": map[string]any{
						"timeout": "10s",
					},
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
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces, invalid verbosity (ignored)",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				Metrics: map[string]any{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
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
					"batch": map[string]any{
						"timeout": "10s",
					},
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
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer"},
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
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				Metrics: map[string]any{
					"delta_ttl":                              1500,
					"resource_attributes_as_tags":            false,
					"instrumentation_scope_metadata_as_tags": false,
					"histograms": map[string]any{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
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
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"batch": map[string]any{
						"timeout": "10s",
					},
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
					},
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer"},
						},
					},
				},
			},
		},
		{
			name: "only gRPC, only Traces, logging with normal verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:          5003,
				TracesEnabled:      true,
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
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				Metrics: map[string]any{
					"delta_ttl":                   1500,
					"resource_attributes_as_tags": false,
					"histograms": map[string]any{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
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
					"batch": map[string]any{
						"timeout": "10s",
					},
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
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces, logging with basic verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				Metrics: map[string]any{
					"delta_ttl":                   2000,
					"resource_attributes_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
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
					"batch": map[string]any{
						"timeout": "10s",
					},
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
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only gRPC, traces and logs",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:          5003,
				TracesEnabled:      true,
				LogsEnabled:        true,
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
					"batch": map[string]any{
						"timeout": "10s",
					},
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
					"logsagent": any(nil),
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
							"processors": []any{"infraattributes", "batch"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP; metrics, logs and traces",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				LogsEnabled:        true,
				Metrics: map[string]any{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
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
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
					"batch": map[string]any{
						"timeout": "10s",
					},
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
					},
					"logsagent": any(nil),
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
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes", "batch"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP; metrics, logs and traces; invalid verbosity (ignored)",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				LogsEnabled:        true,
				Metrics: map[string]any{
					"delta_ttl":                              2000,
					"resource_attributes_as_tags":            true,
					"instrumentation_scope_metadata_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
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
					"batch": map[string]any{
						"timeout": "10s",
					},
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
					},
					"logsagent": any(nil),
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
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes", "batch"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "traces and logs, with both gRPC and HTTP",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 5678),
				TracePort:          5003,
				TracesEnabled:      true,
				LogsEnabled:        true,
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
					"batch": map[string]any{
						"timeout": "10s",
					},
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
					"logsagent": any(nil),
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
							"processors": []any{"infraattributes", "batch"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and logs",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				LogsEnabled:        true,
				Metrics: map[string]any{
					"delta_ttl":                              1500,
					"resource_attributes_as_tags":            false,
					"instrumentation_scope_metadata_as_tags": false,
					"histograms": map[string]any{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
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
							"http": map[string]any{
								"endpoint": "bindhost:1234",
							},
						},
					},
				},
				"processors": map[string]any{
					"infraattributes": any(nil),
					"batch": map[string]any{
						"timeout": "10s",
					},
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
					},
					"logsagent": any(nil),
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes", "batch"},
							"exporters":  []any{"logsagent"},
						},
					},
				},
			},
		},
		{
			name: "only gRPC, traces and logs, logging with normal verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 1234, 0),
				TracePort:          5003,
				TracesEnabled:      true,
				LogsEnabled:        true,
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
					"batch": map[string]any{
						"timeout": "10s",
					},
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
					"logsagent": any(nil),
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
							"processors": []any{"infraattributes", "batch"},
							"exporters":  []any{"logsagent", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and logs, logging with detailed verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				MetricsEnabled:     true,
				LogsEnabled:        true,
				Metrics: map[string]any{
					"delta_ttl":                   1500,
					"resource_attributes_as_tags": false,
					"histograms": map[string]any{
						"mode":                   "nobuckets",
						"send_count_sum_metrics": true,
					},
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
					"batch": map[string]any{
						"timeout": "10s",
					},
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
					},
					"debug": map[string]any{
						"verbosity": "detailed",
					},
					"logsagent": any(nil),
				},
				"service": map[string]any{
					"telemetry": map[string]any{"metrics": map[string]any{"level": "none"}},
					"pipelines": map[string]any{
						"metrics": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer", "debug"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes", "batch"},
							"exporters":  []any{"logsagent", "debug"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP; metrics, traces, and logs; logging with basic verbosity",
			pcfg: PipelineConfig{
				OTLPReceiverConfig: testutil.OTLPConfigFromPorts("bindhost", 0, 1234),
				TracePort:          5003,
				TracesEnabled:      true,
				MetricsEnabled:     true,
				LogsEnabled:        true,
				Metrics: map[string]any{
					"delta_ttl":                   2000,
					"resource_attributes_as_tags": true,
					"histograms": map[string]any{
						"mode":                   "counters",
						"send_count_sum_metrics": true,
					},
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
					"batch": map[string]any{
						"timeout": "10s",
					},
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
					},
					"debug": map[string]any{
						"verbosity": "basic",
					},
					"logsagent": any(nil),
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
							"processors": []any{"batch", "infraattributes"},
							"exporters":  []any{"serializer", "debug"},
						},
						"logs": map[string]any{
							"receivers":  []any{"otlp"},
							"processors": []any{"infraattributes", "batch"},
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
		OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 4317, 4318),
		TracePort:          5001,
		MetricsEnabled:     true,
		TracesEnabled:      true,
		LogsEnabled:        true,
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

	_, err = provider.Get(context.Background(), components)
	require.NoError(t, err)
}

// TestIoTConfigurationOptimization tests that IoT agent uses optimized configurations
func TestIoTConfigurationOptimization(t *testing.T) {
	// Note: Since flavor.GetFlavor() is global state, these tests verify the
	// configuration content rather than runtime behavior

	tests := []struct {
		name               string
		configFunc         func() string
		expectInfraAttribs bool
		description        string
	}{
		{
			name:               "Standard Traces Config",
			configFunc:         func() string { return defaultTracesConfig },
			expectInfraAttribs: true,
			description:        "Standard config should include infraattributes processor",
		},
		{
			name:               "IoT Traces Config",
			configFunc:         func() string { return defaultTracesConfigIoT },
			expectInfraAttribs: false,
			description:        "IoT config should exclude infraattributes processor",
		},
		{
			name:               "Standard Metrics Config",
			configFunc:         func() string { return defaultMetricsConfig },
			expectInfraAttribs: true,
			description:        "Standard metrics config should include infraattributes processor",
		},
		{
			name:               "IoT Metrics Config",
			configFunc:         func() string { return defaultMetricsConfigIoT },
			expectInfraAttribs: false,
			description:        "IoT metrics config should exclude infraattributes processor",
		},
		{
			name:               "Standard Logs Config",
			configFunc:         func() string { return defaultLogsConfig },
			expectInfraAttribs: true,
			description:        "Standard logs config should include infraattributes processor",
		},
		{
			name:               "IoT Logs Config",
			configFunc:         func() string { return defaultLogsConfigIoT },
			expectInfraAttribs: false,
			description:        "IoT logs config should exclude infraattributes processor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configString := tt.configFunc()

			// Parse the YAML configuration
			baseMap, err := configutils.NewMapFromYAMLString(configString)
			require.NoError(t, err, "Failed to parse config: %s", tt.description)

			// Check for infraattributes processor in pipelines
			pipelines, ok := baseMap.Get("service").(map[string]any)
			require.True(t, ok, "Config should have service section")

			pipelineConfig, ok := pipelines["pipelines"].(map[string]any)
			require.True(t, ok, "Service should have pipelines section")

			foundInfraAttribs := false

			// Check all pipeline types (traces, metrics, logs)
			for pipelineType, pipelineData := range pipelineConfig {
				if pipeline, ok := pipelineData.(map[string]any); ok {
					if processors, ok := pipeline["processors"].([]any); ok {
						for _, processor := range processors {
							if processorName, ok := processor.(string); ok && processorName == "infraattributes" {
								foundInfraAttribs = true
								t.Logf("Found infraattributes processor in %s pipeline", pipelineType)
								break
							}
						}
					}
				}
			}

			if tt.expectInfraAttribs {
				assert.True(t, foundInfraAttribs, "%s: %s", tt.name, tt.description)
			} else {
				assert.False(t, foundInfraAttribs, "%s: %s", tt.name, tt.description)
			}

			// Verify that essential components are always present
			assert.Contains(t, configString, "otlp:", "Config should always contain OTLP receiver")
			// Note: batch processor is only in metrics and logs pipelines, not traces
		})
	}
}

// TestGetConfigFunctions tests the dynamic configuration selection functions
func TestGetConfigFunctions(t *testing.T) {
	// Test that the helper functions return appropriate configs
	// Note: Since flavor is global state, we test the function behavior
	// rather than mocking the flavor

	// Test that functions return valid YAML
	configs := []struct {
		name   string
		config string
	}{
		{"getTracesConfig", getTracesConfig()},
		{"getMetricsConfig", getMetricsConfig()},
		{"getLogsConfig", getLogsConfig()},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			// Verify it's valid YAML that can be parsed
			_, err := configutils.NewMapFromYAMLString(tc.config)
			assert.NoError(t, err, "%s should return valid YAML", tc.name)

			// Verify it contains expected basic structure
			assert.Contains(t, tc.config, "receivers:", "%s should contain receivers section", tc.name)
			assert.Contains(t, tc.config, "service:", "%s should contain service section", tc.name)
			assert.Contains(t, tc.config, "pipelines:", "%s should contain pipelines section", tc.name)
		})
	}
}

// TestIoTConfigurationSimplification tests that IoT configs are actually simpler
func TestIoTConfigurationSimplification(t *testing.T) {
	// Compare standard vs IoT configurations to ensure IoT versions are simpler

	comparisons := []struct {
		name        string
		standard    string
		iot         string
		description string
	}{
		{
			name:        "Traces Configuration",
			standard:    defaultTracesConfig,
			iot:         defaultTracesConfigIoT,
			description: "IoT traces config should be simpler than standard",
		},
		{
			name:        "Metrics Configuration",
			standard:    defaultMetricsConfig,
			iot:         defaultMetricsConfigIoT,
			description: "IoT metrics config should be simpler than standard",
		},
		{
			name:        "Logs Configuration",
			standard:    defaultLogsConfig,
			iot:         defaultLogsConfigIoT,
			description: "IoT logs config should be simpler than standard",
		},
	}

	for _, tc := range comparisons {
		t.Run(tc.name, func(t *testing.T) {
			// IoT config should be shorter (fewer characters/lines)
			assert.Less(t, len(tc.iot), len(tc.standard),
				"%s: IoT config should be shorter than standard config", tc.description)

			// IoT config should have fewer processors
			standardProcessorCount := countOccurrences(tc.standard, "processors:")
			iotProcessorCount := countOccurrences(tc.iot, "processors:")

			// Both should have processors section, but IoT might have simpler processor lists
			assert.GreaterOrEqual(t, standardProcessorCount, iotProcessorCount,
				"%s: Standard config should have same or more processor sections", tc.description)
		})
	}
}

// Helper function to count occurrences of a substring
func countOccurrences(text, substring string) int {
	count := 0
	start := 0
	for {
		pos := strings.Index(text[start:], substring)
		if pos == -1 {
			break
		}
		count++
		start += pos + len(substring)
	}
	return count
}
