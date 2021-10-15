// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test
// +build test

package otlp

import (
	"context"
	"strings"
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
		ocfg string
		cfg  map[string]interface{}
	}{
		{
			name: "only gRPC, only Traces",
			pcfg: PipelineConfig{
				GRPCPort:      1234,
				TracePort:     5003,
				BindHost:      "bindhost",
				TracesEnabled: true,
			},
			ocfg: `
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: bindhost:1234
exporters:
  otlp:
    tls:
      insecure: true
    endpoint: localhost:5003
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp]
`,
		},
		{
			name: "only HTTP, metrics and traces",
			pcfg: PipelineConfig{
				HTTPPort:       1234,
				TracePort:      5003,
				BindHost:       "bindhost",
				TracesEnabled:  true,
				MetricsEnabled: true,
			},
			ocfg: `
receivers:
  otlp:
    protocols:
      http:
        endpoint: bindhost:1234

processors:
  batch:

exporters:
  otlp:
    tls:
      insecure: true
    endpoint: localhost:5003
  serializer:

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [serializer]
`,
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
			ocfg: `
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: bindhost:1234
      http:
        endpoint: bindhost:5678
exporters:
  otlp:
    tls:
      insecure: true
    endpoint: localhost:5003
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [otlp]
`,
		},
		{
			name: "only HTTP, only metrics",
			pcfg: PipelineConfig{
				HTTPPort:       1234,
				TracePort:      5003,
				BindHost:       "bindhost",
				MetricsEnabled: true,
			},
			ocfg: `
receivers:
  otlp:
    protocols:
      http:
        endpoint: bindhost:1234

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
`,
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			cfgProvider := newMapProvider(testInstance.pcfg)
			cfg, err := cfgProvider.Get(context.Background())
			require.NoError(t, err)
			tcfg, err := config.NewMapFromBuffer(strings.NewReader(testInstance.ocfg))
			require.NoError(t, err)
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
	})
	configMap, err := mapProvider.Get(context.Background())
	require.NoError(t, err)

	components, err := getComponents(&serializer.MockSerializer{})
	require.NoError(t, err)

	cu := configunmarshaler.NewDefault()
	_, err = cu.Unmarshal(configMap, components)
	require.NoError(t, err)
}
