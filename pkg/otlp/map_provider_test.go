// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test
// +build test

package otlp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configunmarshaler"

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
				GRPCPort:      1234,
				TracePort:     5003,
				BindHost:      "bindhost",
				TracesEnabled: true,
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
						"endpoint": "localhost:5003",
					},
				},
				"service": map[string]interface{}{
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"otlp"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, metrics and traces",
			pcfg: PipelineConfig{
				HTTPPort:       1234,
				TracePort:      5003,
				BindHost:       "bindhost",
				TracesEnabled:  true,
				MetricsEnabled: true,
				Metrics: MetricsConfig{
					DeltaTTL:      2000,
					Quantiles:     false,
					SendMonotonic: true,
					ExporterConfig: MetricsExporterConfig{
						ResourceAttributesAsTags:             true,
						InstrumentationLibraryMetadataAsTags: true,
					},
					HistConfig: HistogramConfig{
						Mode:         "counters",
						SendCountSum: true,
					},
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
					"batch": nil,
				},
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"endpoint": "localhost:5003",
					},
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                                int64(2000),
							"report_quantiles":                         false,
							"send_monotonic_counter":                   true,
							"resource_attributes_as_tags":              true,
							"instrumentation_library_metadata_as_tags": true,
							"histograms": map[string]interface{}{
								"mode":                   "counters",
								"send_count_sum_metrics": true,
							},
						},
					},
				},
				"service": map[string]interface{}{
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"otlp"},
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
				GRPCPort:      1234,
				HTTPPort:      5678,
				TracePort:     5003,
				BindHost:      "bindhost",
				TracesEnabled: true,
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
				"exporters": map[string]interface{}{
					"otlp": map[string]interface{}{
						"tls": map[string]interface{}{
							"insecure": true,
						},
						"endpoint": "localhost:5003",
					},
				},
				"service": map[string]interface{}{
					"pipelines": map[string]interface{}{
						"traces": map[string]interface{}{
							"receivers": []interface{}{"otlp"},
							"exporters": []interface{}{"otlp"},
						},
					},
				},
			},
		},
		{
			name: "only HTTP, only metrics",
			pcfg: PipelineConfig{
				HTTPPort:       1234,
				TracePort:      5003,
				BindHost:       "bindhost",
				MetricsEnabled: true,
				Metrics: MetricsConfig{
					DeltaTTL:      1500,
					Quantiles:     true,
					SendMonotonic: false,
					ExporterConfig: MetricsExporterConfig{
						ResourceAttributesAsTags:             false,
						InstrumentationLibraryMetadataAsTags: false,
					},
					HistConfig: HistogramConfig{
						Mode:         "nobuckets",
						SendCountSum: true,
					},
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
					"batch": nil,
				},
				"exporters": map[string]interface{}{
					"serializer": map[string]interface{}{
						"metrics": map[string]interface{}{
							"delta_ttl":                                int64(1500),
							"report_quantiles":                         true,
							"send_monotonic_counter":                   false,
							"resource_attributes_as_tags":              false,
							"instrumentation_library_metadata_as_tags": false,
							"histograms": map[string]interface{}{
								"mode":                   "nobuckets",
								"send_count_sum_metrics": true,
							},
						},
					},
				},
				"service": map[string]interface{}{
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
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			cfgProvider := newMapProvider(testInstance.pcfg)
			cfg, err := cfgProvider.Get(context.Background())
			require.NoError(t, err)
			tcfg := config.NewMapFromStringMap(testInstance.ocfg)
			assert.Equal(t, tcfg.ToStringMap(), cfg.ToStringMap())
		})
	}
}

func TestUnmarshal(t *testing.T) {
	mapProvider := newMapProvider(PipelineConfig{
		GRPCPort:       4317,
		HTTPPort:       4318,
		TracePort:      5001,
		BindHost:       "localhost",
		MetricsEnabled: true,
		TracesEnabled:  true,
		Metrics: MetricsConfig{
			DeltaTTL:      2000,
			Quantiles:     false,
			SendMonotonic: true,
			ExporterConfig: MetricsExporterConfig{
				ResourceAttributesAsTags:             true,
				InstrumentationLibraryMetadataAsTags: true,
			},
			HistConfig: HistogramConfig{
				Mode:         "counters",
				SendCountSum: true,
			},
		},
	})
	configMap, err := mapProvider.Get(context.Background())
	require.NoError(t, err)

	components, err := getComponents(&serializer.MockSerializer{})
	require.NoError(t, err)

	cu := configunmarshaler.NewDefault()
	_, err = cu.Unmarshal(configMap, components)
	require.NoError(t, err)
}
