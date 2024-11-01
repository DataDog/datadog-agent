// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp && test

package otlp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

func TestGetComponents(t *testing.T) {
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	_, err := getComponents(serializermock.NewMetricSerializer(t), make(chan *message.Message), fakeTagger)
	// No duplicate component
	require.NoError(t, err)
}

func AssertSucessfulRun(t *testing.T, pcfg PipelineConfig) {
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	p, err := NewPipeline(pcfg, serializermock.NewMetricSerializer(t), make(chan *message.Message), fakeTagger)
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
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	p, err := NewPipeline(pcfg, serializermock.NewMetricSerializer(t), make(chan *message.Message), fakeTagger)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pipelineError := p.Run(ctx)
	assert.ErrorContains(t, pipelineError, expected)
}

func TestStartPipeline(t *testing.T) {
	pkgconfigsetup.Datadog().SetWithoutSource("hostname", "otlp-testhostname")
	defer pkgconfigsetup.Datadog().SetWithoutSource("hostname", "")

	pcfg := getTestPipelineConfig()
	AssertSucessfulRun(t, pcfg)
}

func TestStartPipelineFromConfig(t *testing.T) {
	pkgconfigsetup.Datadog().SetWithoutSource("hostname", "otlp-testhostname")
	defer pkgconfigsetup.Datadog().SetWithoutSource("hostname", "")

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
			err:  "'protocols' has invalid keys: htttp",
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.path, func(t *testing.T) {
			cfg, err := testutil.LoadConfig(t, "./testdata/"+testInstance.path)
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

func TestRecoverPanic(t *testing.T) {
	panicTest := func(v any) {
		defer recoverAndStoreError()
		panic(v)
	}
	require.NotPanics(t, func() {
		panicTest("this is a test")
	})
	assert.EqualError(t, pipelineError.Load(), "OTLP pipeline had a panic: this is a test")
}
