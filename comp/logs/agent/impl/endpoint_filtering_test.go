// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !serverless

package agentimpl

import (
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
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
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
	defaultProvider := buildPipelineProvider(agent, nil, nil, diag, destCtx)
	_, isFiltered := defaultProvider.(*pipeline.EndpointFilteredProvider)
	assert.False(t, isFiltered, "expected default provider when no endpoint selectors are configured")

	// With a selector configured, buildPipelineProvider selects EndpointFilteredProvider.
	agent.endpointSelectors = []routing.EndpointSelector{
		nil,
		routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil }),
	}
	filteredProvider := buildPipelineProvider(agent, nil, nil, diag, destCtx)
	efp, isFiltered := filteredProvider.(*pipeline.EndpointFilteredProvider)
	require.True(t, isFiltered, "expected EndpointFilteredProvider when endpoint selectors are configured")
	efp.Stop()

	// Stub compiler rejects non-empty expressions until OTTL integration lands.
	_, err := agent.endpointSelectorCompiler.Compile(`status == "error"`)
	assert.ErrorIs(t, err, routing.ErrSelectorCompilerNotConfigured)
}
