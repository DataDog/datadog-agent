// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretsnoop "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditornone "github.com/DataDog/datadog-agent/comp/logs/auditor/impl-none"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestBuildPipelineProviderSelectsEndpointFilteredWhenSelectorsConfigured(t *testing.T) {
	cfg := configmock.New(t)
	us1 := config.NewEndpoint("key-us1", "", "us1.example.com", 443, "", true)
	us5 := config.NewMockEndpointWithOptions(map[string]any{
		"api_key":     "key-us5",
		"host":        "us5.example.com",
		"port":        443,
		"use_ssl":     true,
		"is_reliable": false,
	})
	eps := config.NewEndpointsWithBatchSettings(us1, []config.Endpoint{us5}, false, true, time.Second, 1, 10, 1024*1024, 10)

	agent := &logAgent{
		config:                   cfg,
		endpoints:                eps,
		auditor:                  auditornone.NewAuditor(),
		compression:              logscompression.NewMockCompressor(),
		secrets:                  secretsnoop.NewComponent().Comp,
		endpointSelectorCompiler: routing.StubSelectorCompiler{},
	}

	diag := diagnostic.NewBufferedMessageReceiver(nil, nil, cfg)
	destCtx := client.NewDestinationsContext()

	// Without selectors, buildPipelineProvider keeps the default provider.
	defaultProvider, err := buildPipelineProvider(agent, nil, nil, diag, destCtx)
	require.NoError(t, err)
	_, isFiltered := defaultProvider.(*pipeline.EndpointFilteredProvider)
	assert.False(t, isFiltered, "expected default provider when no endpoint selectors are configured")

	// With a selector configured, buildPipelineProvider selects EndpointFilteredProvider.
	agent.endpointSelectors = []routing.EndpointSelector{
		nil,
		routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil }),
	}
	filteredProvider, err := buildPipelineProvider(agent, nil, nil, diag, destCtx)
	require.NoError(t, err)
	efp, isFiltered := filteredProvider.(*pipeline.EndpointFilteredProvider)
	require.True(t, isFiltered, "expected EndpointFilteredProvider when endpoint selectors are configured")
	defer efp.Stop()

	// launcherRegistry wraps direct checkpoint writes through GuardRegistry.
	rawReg := auditorMock.NewMockRegistry()
	guarded := launcherRegistry(filteredProvider, rawReg)
	guarded.SetOffset("winevt:sec", "10")
	assert.Equal(t, "10", rawReg.GetOffset("winevt:sec"), "initial write before records are admitted is allowed")

	// Once a replacement generation is registered, direct SetOffset writes are blocked.
	_, err = efp.SourceInput("winevt:sec", 1)
	require.NoError(t, err)
	_, err = efp.SourceInput("winevt:sec", 2)
	require.NoError(t, err)
	guarded.SetOffset("winevt:sec", "999")
	assert.Equal(t, "10", rawReg.GetOffset("winevt:sec"), "direct SetOffset must not bypass GuardRegistry after replacement producer")
	_ = efp.RetireSource(context.Background(), "winevt:sec", 1)
	_ = efp.RetireSource(context.Background(), "winevt:sec", 2)

	// Stub compiler rejects non-empty expressions until OTTL integration lands.
	_, err = agent.endpointSelectorCompiler.Compile(`status == "error"`)
	assert.ErrorIs(t, err, routing.ErrSelectorCompilerNotConfigured)
}

func TestBuildPipelineProviderRejectsInvalidSelectorConfigWithoutUnfilteredFallback(t *testing.T) {
	cfg := configmock.New(t)
	us1 := config.NewEndpoint("key-us1", "", "us1.example.com", 443, "", true)
	sel := routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil })
	diag := diagnostic.NewBufferedMessageReceiver(nil, nil, cfg)
	destCtx := client.NewDestinationsContext()

	// Non-HTTP (TCP) endpoints with explicit selectors must fail instead of falling back.
	tcpAgent := &logAgent{
		config:            cfg,
		endpoints:         config.NewEndpointsWithBatchSettings(us1, nil, false, false, time.Second, 1, 10, 1024*1024, 10),
		auditor:           auditornone.NewAuditor(),
		compression:       logscompression.NewMockCompressor(),
		secrets:           secretsnoop.NewComponent().Comp,
		endpointSelectors: []routing.EndpointSelector{sel},
	}
	prov, err := buildPipelineProvider(tcpAgent, nil, nil, diag, destCtx)
	assert.Nil(t, prov)
	assert.ErrorIs(t, err, routing.ErrInvalidEndpointFilterPolicy)

	// More selectors than endpoints must fail instead of falling back to unfiltered delivery.
	tooManyAgent := &logAgent{
		config:            cfg,
		endpoints:         config.NewEndpointsWithBatchSettings(us1, nil, false, true, time.Second, 1, 10, 1024*1024, 10),
		auditor:           auditornone.NewAuditor(),
		compression:       logscompression.NewMockCompressor(),
		secrets:           secretsnoop.NewComponent().Comp,
		endpointSelectors: []routing.EndpointSelector{nil, sel},
	}
	prov, err = buildPipelineProvider(tooManyAgent, nil, nil, diag, destCtx)
	assert.Nil(t, prov)
	assert.ErrorIs(t, err, routing.ErrInvalidEndpointFilterPolicy)

	// Unused imports guard.
	_ = message.NewOrigin(sources.NewLogSource("test", &config.LogsConfig{}))
}
