// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

package traceconfigimpl

// team: agent-apm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
