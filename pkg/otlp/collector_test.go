// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test
// +build test

package otlp

import (
	"context"
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

	assert.Equal(t, service.Starting, <-p.col.GetStateChannel())
	assert.Equal(t, service.Running, <-p.col.GetStateChannel())

	p.Stop()
	p.Stop()
	<-colDone
	assert.Equal(t, service.Closing, <-p.col.GetStateChannel())
	assert.Equal(t, service.Closed, <-p.col.GetStateChannel())
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
	tests := []struct {
		path string
		err  string
	}{
		{path: "port/nobindhost.yaml"},
		{path: "port/nonlocal.yaml"},
		{
			path: "receiver/noprotocols.yaml",
			err:  "cannot load configuration: error reading receivers configuration for otlp: empty config for OTLP receiver",
		},
		{path: "receiver/simple.yaml"},
		{path: "receiver/advanced.yaml"},
		{
			path: "receiver/typo.yaml",
			err: `cannot load configuration: error reading receivers configuration for otlp: 1 error(s) decoding:

* 'protocols' has invalid keys: htttp`,
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
