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

func TestStartPipeline(t *testing.T) {
	pcfg := PipelineConfig{
		OTLPReceiverConfig: testutil.OTLPConfigFromPorts("localhost", 4317, 4318),
		TracePort:          5003,
		MetricsEnabled:     true,
		TracesEnabled:      true,
		Metrics:            map[string]interface{}{},
	}

	p, err := NewPipeline(pcfg, &serializer.MockSerializer{})
	require.NoError(t, err)

	colDone := make(chan struct{})
	go func() {
		defer close(colDone)
		require.NoError(t, p.Run(context.Background()))
	}()

	assert.Equal(t, service.Starting, <-p.col.GetStateChannel())
	assert.Equal(t, service.Running, <-p.col.GetStateChannel())

	p.Stop()
	p.Stop()
	<-colDone
	assert.Equal(t, service.Closing, <-p.col.GetStateChannel())
	assert.Equal(t, service.Closed, <-p.col.GetStateChannel())

}
