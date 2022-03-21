// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && test
// +build otlp,test

package otlp

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/service"

	"github.com/DataDog/datadog-agent/pkg/otlp/internal/testutil"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func TestGetComponents(t *testing.T) {
	_, err := getComponents(&serializer.MockSerializer{})
	// No duplicate component
	require.NoError(t, err)
}

func AssertSucessfulRun(t *testing.T, pcfg PipelineConfig) {
	p, err := NewPipeline(pcfg, &serializer.MockSerializer{})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	colDone := make(chan struct{})
	go func() {
		defer close(colDone)
		require.NoError(t, p.Run(ctx))
	}()

	assert.Eventually(t, func() bool {
		return service.Running == p.col.GetState()
	}, time.Second*2, time.Millisecond*200)

	p.Stop()
	p.Stop()
	<-colDone

	assert.Eventually(t, func() bool {
		return service.Closed == p.col.GetState()
	}, time.Second*2, time.Millisecond*200)
}

func AssertFailedRun(t *testing.T, pcfg PipelineConfig, expected string) {
	p, err := NewPipeline(pcfg, &serializer.MockSerializer{})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	assert.EqualError(t, p.Run(ctx), expected)
}

func TestStartPipeline(t *testing.T) {
	pcfg := PipelineConfig{
		OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 4317, 4318),
		TracePort:          5003,
		MetricsEnabled:     true,
		TracesEnabled:      true,
		Metrics:            map[string]interface{}{},
	}
	AssertSucessfulRun(t, pcfg)
}

func TestStartPipelineFromConfig(t *testing.T) {
	// TODO (AP-1550): Fix this once we can disable changing the gRPC logger
	if runtime.GOOS == "windows" {
		t.Skip("Skip on Windows, see AP-1550 for details")
	}

	tests := []struct {
		path string
		err  string
	}{
		{path: "experimental/port/nobindhost.yaml"},
		{path: "experimental/port/nonlocal.yaml"},
		{
			path: "experimental/receiver/noprotocols.yaml",
			err:  "failed to get config: cannot unmarshal the configuration: error reading receivers configuration for \"otlp\": empty config for OTLP receiver",
		},
		{path: "experimental/receiver/simple.yaml"},
		{path: "experimental/receiver/advanced.yaml"},
		{
			path: "experimental/receiver/typo.yaml",
			err:  "failed to get config: cannot unmarshal the configuration: error reading receivers configuration for \"otlp\": 1 error(s) decoding:\n\n* 'protocols' has invalid keys: htttp",
		},

		{
			path: "stable/receiver/noprotocols.yaml",
			err:  "failed to get config: cannot unmarshal the configuration: error reading receivers configuration for \"otlp\": empty config for OTLP receiver",
		},
		{path: "stable/receiver/simple.yaml"},
		{path: "stable/receiver/advanced.yaml"},
		{
			path: "stable/receiver/typo.yaml",
			err:  "failed to get config: cannot unmarshal the configuration: error reading receivers configuration for \"otlp\": 1 error(s) decoding:\n\n* 'protocols' has invalid keys: htttp",
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig("./testdata/" + testInstance.path)
			require.NoError(t, err)
			pcfg, err := FromAgentConfig(cfg)
			require.NoError(t, err)
			if testInstance.err == "" {
				AssertSucessfulRun(t, pcfg)
			} else {
				AssertFailedRun(t, pcfg, testInstance.err)
			}
		})
	}
}
