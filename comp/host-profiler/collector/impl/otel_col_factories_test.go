// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package collectorimpl

import (
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/attributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtraFactoriesWithoutAgentCore_GetProcessors(t *testing.T) {
	extraFactories := NewExtraFactoriesWithoutAgentCore()
	factories, err := createFactories(extraFactories)()
	require.NoError(t, err)

	processors := factories.Processors
	_, found := processors[k8sattributesprocessor.NewFactory().Type()]
	require.True(t, found)

	_, found = processors[attributesprocessor.NewFactory().Type()]
	require.True(t, found)
}

func TestExtraFactoriesWithoutAgentCore_GetReceivers(t *testing.T) {
	extraFactories := NewExtraFactoriesWithoutAgentCore()
	factories, err := createFactories(extraFactories)()
	require.NoError(t, err)

	_, found := factories.Receivers[filelogreceiver.NewFactory().Type()]
	assert.True(t, found, "filelog receiver should be registered for standalone mode")
}

func TestExtraFactoriesWithAgentCore_GetReceivers(t *testing.T) {
	extra := extraFactoriesWithAgentCore{}
	assert.Nil(t, extra.GetReceivers(), "agent core mode should not add extra receivers")
}
