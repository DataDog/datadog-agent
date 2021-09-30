// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package otlp

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/config/configparser"
	"go.opentelemetry.io/collector/service/parserprovider"
)

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

// buildKey creates a key for use in the ConfigMap.Set function.
func buildKey(keys ...string) string {
	return strings.Join(keys, configparser.KeyDelimiter)
}

// newMap creates a configparser.ConfigMap with the fixed configuration.
// TODO (AP-1254): Refactor with MergeProvider when available.
func newMap(cfg PipelineConfig) (*configparser.ConfigMap, error) {
	configMap := configparser.NewConfigMap()

	if cfg.TracesEnabled {
		tracesMap, err := configparser.NewConfigMapFromBuffer(strings.NewReader(defaultTracesConfig))
		if err != nil {
			return nil, err
		}

		err = configMap.MergeStringMap(tracesMap.ToStringMap())
		if err != nil {
			return nil, fmt.Errorf("failed to merge traces map: %w", err)
		}

		configMap.Set(
			buildKey("exporters", "otlp", "endpoint"),
			fmt.Sprintf("%s:%d", "localhost", cfg.TracePort),
		)
	}

	if cfg.MetricsEnabled {
		metricsMap, err := configparser.NewConfigMapFromBuffer(strings.NewReader(defaultMetricsConfig))
		if err != nil {
			return nil, err
		}

		err = configMap.MergeStringMap(metricsMap.ToStringMap())
		if err != nil {
			return nil, fmt.Errorf("failed to merge metrics map: %w", err)
		}
	}

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

	return configMap, nil
}

// TODO(AP-1254): Use a  InMemory provider instead of this.
var _ parserprovider.ParserProvider = (*parserProvider)(nil)

type parserProvider configparser.ConfigMap

func (p parserProvider) Get(context.Context) (*configparser.ConfigMap, error) {
	return (*configparser.ConfigMap)(&p), nil
}

func (p parserProvider) Close(context.Context) error {
	return nil
}
