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

// baseConfig is the base OTLP pipeline configuration.
// This pipeline is extended through the datadog.yaml configuration values.
// It is written in YAML because it is easier to read and write than a map.
// TODO (AP-1254): Set service-level configuration when available.
const baseConfig string = `
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

// buildKey creates a key for use in the ConfigMap.Set function.
func buildKey(keys ...string) string {
	return strings.Join(keys, configparser.KeyDelimiter)
}

// newParser creates a configparser.ConfigMap with the fixed configuration.
func newParser(cfg PipelineConfig) (*configparser.ConfigMap, error) {
	parser, err := configparser.NewConfigMapFromBuffer(strings.NewReader(baseConfig))
	if err != nil {
		return nil, err
	}

	if cfg.GRPCPort > 0 {
		parser.Set(
			buildKey("receivers", "otlp", "protocols", "grpc", "endpoint"),
			fmt.Sprintf("%s:%d", cfg.BindHost, cfg.GRPCPort),
		)
	}

	if cfg.HTTPPort > 0 {
		parser.Set(
			buildKey("receivers", "otlp", "protocols", "http", "endpoint"),
			fmt.Sprintf("%s:%d", cfg.BindHost, cfg.HTTPPort),
		)
	}

	parser.Set(
		buildKey("exporters", "otlp", "endpoint"),
		fmt.Sprintf("%s:%d", "localhost", cfg.TracePort),
	)

	return parser, nil
}

// TODO: Use a  InMemory provider instead of this.
var _ parserprovider.ParserProvider = (*parserProvider)(nil)

type parserProvider configparser.ConfigMap

func (p parserProvider) Get(context.Context) (*configparser.ConfigMap, error) {
	return (*configparser.ConfigMap)(&p), nil
}

func (p parserProvider) Close(context.Context) error {
	return nil
}
