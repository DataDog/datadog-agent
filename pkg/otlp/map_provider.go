// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package otlp

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/service/parserprovider"
)

// buildKey creates a key for use in the config.Map.Set function.
func buildKey(keys ...string) string {
	return strings.Join(keys, config.KeyDelimiter)
}

var _ parserprovider.MapProvider = (*mapProvider)(nil)

type mapProvider config.Map

func (p mapProvider) Get(context.Context) (*config.Map, error) {
	return (*config.Map)(&p), nil
}

func (p mapProvider) Close(context.Context) error {
	return nil
}

// defaultTracesConfig is the base traces OTLP pipeline configuration.
// This pipeline is extended through the datadog.yaml configuration values.
// It is written in YAML because it is easier to read and write than a map.
// TODO (AP-1254): Set service-level configuration when available.
const defaultTracesConfig string = `
receivers:
  otlp:

exporters:
  otlp:
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp]
`

func newTracesMapProvider(tracePort uint) parserprovider.MapProvider {
	configMap := config.NewMap()
	configMap.Set(buildKey("exporters", "otlp", "endpoint"), fmt.Sprintf("%s:%d", "localhost", tracePort))
	return parserprovider.NewMergeMapProvider(
		parserprovider.NewInMemoryMapProvider(strings.NewReader(defaultTracesConfig)),
		mapProvider(*configMap),
	)
}

// defaultMetricsConfig is the metrics OTLP pipeline configuration.
// TODO (AP-1254): Set service-level configuration when available.
const defaultMetricsConfig string = `
receivers:
  otlp:

processors:
  batch:

exporters:
  serializer:

service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [serializer]
`

func newMetricsMapProvider(cfg PipelineConfig) parserprovider.MapProvider {
	configMap := config.NewMap()

	configMap.Set(
		buildKey("exporters", "serializer", "metrics", "delta_ttl"),
		cfg.Metrics.DeltaTTL,
	)

	configMap.Set(
		buildKey("exporters", "serializer", "metrics", "send_monotonic_counter"),
		cfg.Metrics.SendMonotonic,
	)

	configMap.Set(
		buildKey("exporters", "serializer", "metrics", "report_quantiles"),
		cfg.Metrics.Quantiles,
	)

	configMap.Set(
		buildKey("exporters", "serializer", "metrics", "resource_attributes_as_tags"),
		cfg.Metrics.ExporterConfig.ResourceAttributesAsTags,
	)

	configMap.Set(
		buildKey("exporters", "serializer", "metrics", "instrumentation_library_metadata_as_tags"),
		cfg.Metrics.ExporterConfig.InstrumentationLibraryMetadataAsTags,
	)

	configMap.Set(
		buildKey("exporters", "serializer", "metrics", "histograms", "mode"),
		cfg.Metrics.HistConfig.Mode,
	)

	configMap.Set(
		buildKey("exporters", "serializer", "metrics", "histograms", "send_count_sum_metrics"),
		cfg.Metrics.HistConfig.SendCountSum,
	)

	return parserprovider.NewMergeMapProvider(
		parserprovider.NewInMemoryMapProvider(strings.NewReader(defaultMetricsConfig)),
		mapProvider(*configMap),
	)
}

func newReceiverProvider(cfg PipelineConfig) parserprovider.MapProvider {
	configMap := config.NewMap()
	if cfg.GRPCPort > 0 {
		configMap.Set(
			buildKey("receivers", "otlp", "protocols", "grpc", "endpoint"),
			fmt.Sprintf("%s:%d", cfg.BindHost, cfg.GRPCPort),
		)
	}

	if cfg.HTTPPort > 0 {
		configMap.Set(
			buildKey("receivers", "otlp", "protocols", "http", "endpoint"),
			fmt.Sprintf("%s:%d", cfg.BindHost, cfg.HTTPPort),
		)
	}

	return mapProvider(*configMap)
}

// newMapProvider creates a parserprovider.MapProvider with the fixed configuration.
func newMapProvider(cfg PipelineConfig) parserprovider.MapProvider {
	var providers []parserprovider.MapProvider
	if cfg.TracesEnabled {
		providers = append(providers, newTracesMapProvider(cfg.TracePort))
	}
	if cfg.MetricsEnabled {
		providers = append(providers, newMetricsMapProvider(cfg))
	}
	providers = append(providers, newReceiverProvider(cfg))
	return parserprovider.NewMergeMapProvider(providers...)
}
