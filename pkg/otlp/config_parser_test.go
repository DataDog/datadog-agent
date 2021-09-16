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
			tcfg, err := configparser.NewParserFromBuffer(strings.NewReader(testInstance.ocfg))
			require.NoError(t, err)
			assert.Equal(t, tcfg.ToStringMap(), cfg.ToStringMap())
		})
	}
}
