// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

package otlp

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"
	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/pkg/otlp/internal/configutils"
)

// buildKey creates a key for referencing a nested field.
func buildKey(keys ...string) string {
	return strings.Join(keys, confmap.KeyDelimiter)
}

// defaultTracesConfig is the base traces OTLP pipeline configuration.
// This pipeline is extended through the datadog.yaml configuration values.
// It is written in YAML because it is easier to read and write than a map.
const defaultTracesConfig string = `
receivers:
  otlp:

processors:
  batch:
    timeout: 10s

exporters:
  otlp:
    tls:
      insecure: true
    compression: none

service:
  telemetry:
    metrics:
      level: none
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlp]
`

func buildTracesMap(tracePort uint) (*confmap.Conf, error) {
	baseMap, err := configutils.NewMapFromYAMLString(defaultTracesConfig)
	if err != nil {
		return nil, err
	}
	{
		configMap := confmap.NewFromStringMap(map[string]interface{}{
			buildKey("exporters", "otlp", "endpoint"): fmt.Sprintf("%s:%d", "localhost", tracePort),
		})
		err = baseMap.Merge(configMap)
	}
	return baseMap, err
}

// defaultMetricsConfig is the metrics OTLP pipeline configuration.
const defaultMetricsConfig string = `
receivers:
  otlp:

processors:
  batch:
    timeout: 10s

exporters:
  serializer:

service:
  telemetry:
    metrics:
      level: none
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [serializer]
`

func buildMetricsMap(cfg PipelineConfig) (*confmap.Conf, error) {
	baseMap, err := configutils.NewMapFromYAMLString(defaultMetricsConfig)
	if err != nil {
		return nil, err
	}

	{
		configMap := confmap.NewFromStringMap(map[string]interface{}{
			buildKey("exporters", "serializer", "metrics"): cfg.Metrics,
		})
		err = baseMap.Merge(configMap)
	}
	return baseMap, err
}

func buildReceiverMap(otlpReceiverConfig map[string]interface{}) *confmap.Conf {
	return confmap.NewFromStringMap(map[string]interface{}{
		"receivers": map[string]interface{}{"otlp": otlpReceiverConfig},
	})
}

func buildMap(cfg PipelineConfig) (*confmap.Conf, error) {
	retMap := confmap.New()
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
	if cfg.shouldSetLoggingSection() {
		m := map[string]interface{}{
			"exporters": map[string]interface{}{
				"logging": cfg.Debug,
			},
		}
		if cfg.MetricsEnabled {
			key := buildKey("service", "pipelines", "metrics", "exporters")
			if v, ok := retMap.Get(key).([]interface{}); ok {
				m[key] = append(v, "logging")
			} else {
				m[key] = []interface{}{"logging"}
			}
		}
		if cfg.TracesEnabled {
			key := buildKey("service", "pipelines", "traces", "exporters")
			if v, ok := retMap.Get(key).([]interface{}); ok {
				m[key] = append(v, "logging")
			} else {
				m[key] = []interface{}{"logging"}
			}
		}
		errs = append(errs, retMap.Merge(confmap.NewFromStringMap(m)))
	}

	err := retMap.Merge(buildReceiverMap(cfg.OTLPReceiverConfig))
	errs = append(errs, err)

	return retMap, multierr.Combine(errs...)
}

// newMapProvider creates a service.ConfigProvider with the fixed configuration.
func newMapProvider(cfg PipelineConfig) (otelcol.ConfigProvider, error) {
	cfgMap, err := buildMap(cfg)
	if err != nil {
		return nil, err
	}
	return configutils.NewConfigProviderFromMap(cfgMap), nil
}
