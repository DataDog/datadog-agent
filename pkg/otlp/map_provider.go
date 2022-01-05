// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// +build !serverless

package otlp

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configmapprovider"
)

// buildKey creates a key for use in the config.Map.Set function.
func buildKey(keys ...string) string {
	return strings.Join(keys, config.KeyDelimiter)
}

var _ configmapprovider.Provider = (*mapProvider)(nil)
var _ configmapprovider.Retrieved = (*mapProvider)(nil)

type mapProvider config.Map

// TODO: In v0.42.0, direct implementation of Retrieved won't be allowed.
// The new configmapprovider.NewRetrieved helper should be used instead.
// See: https://github.com/open-telemetry/opentelemetry-collector/pull/4577
func (p mapProvider) Get(context.Context) (*config.Map, error) {
	return (*config.Map)(&p), nil
}

func (p mapProvider) Close(context.Context) error {
	return nil
}

func (p mapProvider) Retrieve(context.Context, func(*configmapprovider.ChangeEvent)) (configmapprovider.Retrieved, error) {
	return &p, nil
}

func (p mapProvider) Shutdown(context.Context) error {
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

func newTracesMapProvider(tracePort uint) configmapprovider.Provider {
	configMap := config.NewMap()
	configMap.Set(buildKey("exporters", "otlp", "endpoint"), fmt.Sprintf("%s:%d", "localhost", tracePort))
	return configmapprovider.NewMerge(
		configmapprovider.NewInMemory(strings.NewReader(defaultTracesConfig)),
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
    timeout: 10s

exporters:
  serializer:

service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [serializer]
`

func newMetricsMapProvider(cfg PipelineConfig) configmapprovider.Provider {
	configMap := config.NewMap()

	configMap.Set(
		buildKey("exporters", "serializer", "metrics"),
		cfg.Metrics,
	)

	return configmapprovider.NewMerge(
		configmapprovider.NewInMemory(strings.NewReader(defaultMetricsConfig)),
		mapProvider(*configMap),
	)
}

func newReceiverProvider(otlpReceiverConfig map[string]interface{}) configmapprovider.Provider {
	configMap := config.NewMapFromStringMap(map[string]interface{}{
		"receivers": map[string]interface{}{"otlp": otlpReceiverConfig},
	})
	return mapProvider(*configMap)
}

// newMapProvider creates a config.MapProvider with the fixed configuration.
func newMapProvider(cfg PipelineConfig) configmapprovider.Provider {
	var providers []configmapprovider.Provider
	if cfg.TracesEnabled {
		providers = append(providers, newTracesMapProvider(cfg.TracePort))
	}
	if cfg.MetricsEnabled {
		providers = append(providers, newMetricsMapProvider(cfg))
	}
	providers = append(providers, newReceiverProvider(cfg.OTLPReceiverConfig))
	return configmapprovider.NewMerge(providers...)
}
