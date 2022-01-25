// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// +build !serverless,otlp

package otlp

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configmapprovider"
	"go.opentelemetry.io/collector/config/configunmarshaler"
	"go.opentelemetry.io/collector/service"
	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/pkg/otlp/internal/configutils"
)

// buildKey creates a key for use in the config.Map.Set function.
func buildKey(keys ...string) string {
	return strings.Join(keys, config.KeyDelimiter)
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

func buildTracesMap(tracePort uint) (*config.Map, error) {
	baseMap, err := configutils.NewMapFromYAMLString(defaultTracesConfig)
	if err != nil {
		return nil, err
	}
	{
		configMap := config.NewMap()
		configMap.Set(buildKey("exporters", "otlp", "endpoint"), fmt.Sprintf("%s:%d", "localhost", tracePort))
		err = baseMap.Merge(configMap)
	}
	return baseMap, err
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

func buildMetricsMap(cfg PipelineConfig) (*config.Map, error) {
	baseMap, err := configutils.NewMapFromYAMLString(defaultMetricsConfig)
	if err != nil {
		return nil, err
	}

	{
		configMap := config.NewMap()
		configMap.Set(buildKey("exporters", "serializer", "metrics"), cfg.Metrics)
		err = baseMap.Merge(configMap)
	}
	return baseMap, err
}

func buildReceiverMap(otlpReceiverConfig map[string]interface{}) *config.Map {
	return config.NewMapFromStringMap(map[string]interface{}{
		"receivers": map[string]interface{}{"otlp": otlpReceiverConfig},
	})
}

func buildMap(cfg PipelineConfig) (*config.Map, error) {
	retMap := config.NewMap()
	var errs []error
	if cfg.TracesEnabled {
		traceMap, err := buildTracesMap(cfg.TracePort)
		errs = append(errs, err)

		err = retMap.Merge(traceMap)
		errs = append(errs, err)
	}
	if cfg.MetricsEnabled {
		metricsMap, err := buildMetricsMap(cfg)
		errs = append(errs, err)

		err = retMap.Merge(metricsMap)
		errs = append(errs, err)
	}
	err := retMap.Merge(buildReceiverMap(cfg.OTLPReceiverConfig))
	errs = append(errs, err)

	return retMap, multierr.Combine(errs...)
}

// newMapProvider creates a service.ConfigProvider with the fixed configuration.
func newMapProvider(cfg PipelineConfig) (service.ConfigProvider, error) {
	cfgMap, err := buildMap(cfg)
	if err != nil {
		return nil, err
	}
	provider := configutils.NewProviderFromMap(cfgMap)
	return service.NewConfigProvider(
		[]configmapprovider.Provider{provider},
		[]service.ConfigMapConverterFunc{},
		configunmarshaler.NewDefault(),
	), nil
}
