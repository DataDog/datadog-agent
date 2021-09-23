// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package otlp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configparser"
	"go.opentelemetry.io/collector/config/configunmarshaler"
)

func TestNewParser(t *testing.T) {
	tests := []struct {
		name string
		pcfg PipelineConfig
		ocfg string
		cfg  map[string]interface{}
	}{
		{
			name: "only gRPC",
			pcfg: PipelineConfig{
				GRPCPort:  1234,
				TracePort: 5003,
				BindHost:  "bindhost",
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
			name: "only HTTP",
			pcfg: PipelineConfig{
				HTTPPort:  1234,
				TracePort: 5003,
				BindHost:  "bindhost",
			},
			ocfg: `
receivers:
  otlp:
    protocols:
      http:
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
			name: "with both",
			pcfg: PipelineConfig{
				GRPCPort:  1234,
				HTTPPort:  5678,
				TracePort: 5003,
				BindHost:  "bindhost",
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
	}

	for _, testInstance := range tests {
		t.Run(testInstance.name, func(t *testing.T) {
			cfg, err := newParser(testInstance.pcfg)
			require.NoError(t, err)
			tcfg, err := configparser.NewConfigMapFromBuffer(strings.NewReader(testInstance.ocfg))
			require.NoError(t, err)
			assert.Equal(t, tcfg.ToStringMap(), cfg.ToStringMap())
		})
	}
}

func TestUnmarshal(t *testing.T) {
	configMap, err := newParser(PipelineConfig{
		GRPCPort:  4317,
		HTTPPort:  4318,
		TracePort: 5001,
		BindHost:  "localhost",
	})
	require.NoError(t, err)

	components, err := getComponents()
	require.NoError(t, err)

	cu := configunmarshaler.NewDefault()
	_, err = cu.Unmarshal(configMap, components)
	require.NoError(t, err)
}
