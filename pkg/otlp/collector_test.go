// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && test

package otlp

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/otlp/internal/testutil"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/otelcol"
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
		return otelcol.StateRunning == p.col.GetState()
	}, time.Second*2, time.Millisecond*200)

	p.Stop()
	p.Stop()
	<-colDone

	assert.Eventually(t, func() bool {
		return otelcol.StateClosed == p.col.GetState()
	}, time.Second*2, time.Millisecond*200)
}

func AssertFailedRun(t *testing.T, pcfg PipelineConfig, expected string) {
	p, err := NewPipeline(pcfg, &serializer.MockSerializer{})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	assert.ErrorContains(t, p.Run(ctx), expected)
}

func TestStartPipeline(t *testing.T) {
	config.Datadog.Set("hostname", "otlp-testhostname")
	defer config.Datadog.Set("hostname", "")

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
	config.Datadog.Set("hostname", "otlp-testhostname")
	defer config.Datadog.Set("hostname", "")

	// TODO (AP-1723): Disable changing the gRPC logger before re-enabling.
	if runtime.GOOS == "windows" {
		t.Skip("Skip on Windows, see AP-1723 for details")
	}

	// TODO (AP-1723): Update Collector to version 0.55 before re-enabling.
	if runtime.GOOS == "darwin" {
		t.Skip("Skip on macOS, see AP-1723 for details")
	}

	tests := []struct {
		path string
		err  string
	}{
		{
			path: "receiver/noprotocols.yaml",
			err:  "invalid configuration: receivers::otlp: must specify at least one protocol when using the OTLP receiver",
		},
		{path: "receiver/simple.yaml"},
		{path: "receiver/advanced.yaml"},
		{
			path: "receiver/typo.yaml",
			err:  "error decoding 'receivers': error reading configuration for \"otlp\": 1 error(s) decoding:\n\n* 'protocols' has invalid keys: htttp",
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
