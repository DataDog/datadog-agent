// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package converterimpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConvert_UnsetEnvVarInPrometheusHost is a regression test for OTAGENT-575.
// It reproduces the panic in findInternalMetricsAddress when an unset env var is used
// as the prometheus host. After the fix, Convert must return an error instead of panicking.
func TestConvert_UnsetEnvVarInPrometheusHost(t *testing.T) {
	cfgYAML := `receivers:
  otlp:
    protocols:
      grpc:
exporters:
  datadog:
    api:
      key: test-key
service:
  telemetry:
    metrics:
      readers:
        - pull:
            exporter:
              prometheus:
                host: ${env:OTAGENT575_PROMETHEUS_HOST_TEST}
                port: 8888
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [datadog]
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "otel-config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgYAML), 0600))

	os.Unsetenv("OTAGENT575_PROMETHEUS_HOST_TEST")

	resolver, err := newResolver([]string{cfgPath})
	require.NoError(t, err)
	conf, err := resolver.Resolve(context.Background())
	require.NoError(t, err)

	converter, err := NewConverterForAgent(Requires{})
	require.NoError(t, err)

	var convertErr error
	require.NotPanics(t, func() {
		convertErr = converter.Convert(context.Background(), conf)
	})
	require.Error(t, convertErr)
}
