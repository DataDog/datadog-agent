// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp && test

package pipelineimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/configcheck"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

type recordingInventory map[string]interface{}

func (r recordingInventory) Set(name string, value interface{}) { r[name] = value }

func (r recordingInventory) Get() map[string]interface{} { return r }

func TestStartDataPlaneOTLPModes(t *testing.T) {
	const tlsReceiver = `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
        tls:
          cert_file: /etc/datadog-agent/otlp/cert.pem
          key_file: /etc/datadog-agent/otlp/key.pem
`
	tests := []struct {
		name            string
		proxyTraces     bool
		collectorStatus string
		errContains     string
	}{
		{
			name:            "data plane only does not start the pipeline",
			collectorStatus: "Not running, the data plane handles OTLP ingest",
		},
		{
			name:            "invalid proxy configuration reports the error",
			proxyTraces:     true,
			collectorStatus: "Failed to start",
			errContains:     configcheck.ErrDataPlaneProxyTLS.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_DATA_PLANE_ENABLED", "true")
			t.Setenv("DD_DATA_PLANE_OTLP_ENABLED", "true")
			if tt.proxyTraces {
				t.Setenv("DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_TRACES", "true")
			}
			inventory := recordingInventory{}
			c := &collectorImpl{
				config:         configmock.NewFromYAML(t, tlsReceiver),
				log:            logmock.New(t),
				inventoryAgent: inventory,
				ctx:            context.Background(),
			}

			require.NoError(t, c.start(context.Background()))
			assert.Nil(t, c.col)
			assert.Equal(t, true, inventory[otlpEnabled])

			stats := map[string]interface{}{}
			require.NoError(t, c.JSON(false, stats))
			status, ok := stats["otlp"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, true, status["otlpStatus"])
			assert.Equal(t, tt.collectorStatus, status["otlpCollectorStatus"])
			if tt.errContains == "" {
				assert.Empty(t, status["otlpCollectorStatusErr"])
			} else {
				assert.Contains(t, status["otlpCollectorStatusErr"], tt.errContains)
			}
		})
	}
}
