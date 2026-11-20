// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serializerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	defaultforwardernoop "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/noop-impl"
	orchestratormock "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/mock"
	metricscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func TestNewComponentProvidesSingleInstance(t *testing.T) {
	hostname, _ := hostnamemock.NewMock("test-host")
	provides, err := NewComponent(Requires{
		Config:                config.NewMock(t),
		Log:                   logmock.New(t),
		Forwarder:             defaultforwardernoop.NewComponent(),
		OrchestratorForwarder: orchestratormock.NewMockOrchestratorForwarder(),
		Compressor:            metricscompressionmock.NewMockCompressor(),
		Hostname:              hostname,
	})
	require.NoError(t, err)

	s, ok := provides.Comp.(*serializer.Serializer)
	require.True(t, ok, "Comp must be the concrete *serializer.Serializer")
	assert.Same(t, s, provides.Metrics)
	assert.Same(t, s, provides.Events)
	assert.Same(t, s, provides.Metadata)
	assert.Same(t, s, provides.Orchestrator)
}
