// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package configimpl

// team: agent-apm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
)

func TestFullYamlConfigWithOTLP(t *testing.T) {
	config := buildConfigComponentFromYAML(t, true, "./testdata/full.yaml")
	cfg := config.Object()

	require.NotNil(t, cfg)

	assert.Equal(t, "0.0.0.0", cfg.OTLPReceiver.BindHost)
	assert.Equal(t, 50053, cfg.OTLPReceiver.GRPCPort)
	assert.Equal(t, 0, cfg.OTLPReceiver.GrpcMaxRecvMsgSizeMib)
}

func TestOTLPGrpcMaxRecvMsgSizeMibFromEnv(t *testing.T) {
	t.Setenv("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_MAX_RECV_MSG_SIZE_MIB", "100")
	config := buildConfigComponentFromYAML(t, true, "./testdata/full.yaml")
	cfg := config.Object()

	require.NotNil(t, cfg)

	assert.Equal(t, 100, cfg.OTLPReceiver.GrpcMaxRecvMsgSizeMib)
}

func TestOTLPInternalReceiverDataPlaneModes(t *testing.T) {
	const (
		plaintext = `
apm_config:
  receiver_port: 18126
otlp_config:
  receiver:
    protocols:
      grpc:
  traces:
    internal_port: 50053
`
		withTLS = `
apm_config:
  receiver_port: 18126
otlp_config:
  receiver:
    protocols:
      grpc:
        tls:
          cert_file: /etc/datadog-agent/otlp/cert.pem
          key_file: /etc/datadog-agent/otlp/key.pem
  traces:
    internal_port: 50053
`
	)
	const (
		dataPlaneEnabled = "DD_DATA_PLANE_ENABLED"
		dataPlaneOTLP    = "DD_DATA_PLANE_OTLP_ENABLED"
		proxyMetrics     = "DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_METRICS"
		proxyTraces      = "DD_DATA_PLANE_EXPERIMENTAL_OTLP_PROXY_TRACES"
	)
	tests := []struct {
		name     string
		yaml     string
		env      map[string]string
		grpcPort int
	}{
		{
			name:     "data plane disabled",
			yaml:     withTLS,
			env:      map[string]string{dataPlaneOTLP: "true", proxyTraces: "true"},
			grpcPort: 50053,
		},
		{
			name:     "data plane OTLP disabled",
			yaml:     withTLS,
			env:      map[string]string{dataPlaneEnabled: "true", proxyTraces: "true"},
			grpcPort: 50053,
		},
		{
			name: "data plane only",
			yaml: plaintext,
			env:  map[string]string{dataPlaneEnabled: "true", dataPlaneOTLP: "true"},
		},
		{
			name: "data plane only with TLS",
			yaml: withTLS,
			env:  map[string]string{dataPlaneEnabled: "true", dataPlaneOTLP: "true"},
		},
		{
			name:     "proxy traces",
			yaml:     plaintext,
			env:      map[string]string{dataPlaneEnabled: "true", dataPlaneOTLP: "true", proxyTraces: "true"},
			grpcPort: 50053,
		},
		{
			name:     "proxy metrics only",
			yaml:     plaintext,
			env:      map[string]string{dataPlaneEnabled: "true", dataPlaneOTLP: "true", proxyMetrics: "true"},
			grpcPort: 50053,
		},
		{
			name: "proxy with TLS",
			yaml: withTLS,
			env:  map[string]string{dataPlaneEnabled: "true", dataPlaneOTLP: "true", proxyTraces: "true"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			cfg := buildComponent(t, true, configcomp.NewMockFromYAML(t, tt.yaml)).Object()

			assert.Equal(t, tt.grpcPort, cfg.OTLPReceiver.GRPCPort)
			assert.Equal(t, 18126, cfg.ReceiverPort)
		})
	}
}
