// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestFactoryOrderMainThenAdditional(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443, "is_reliable": true})
	extra := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "extra.example", "port": 443, "is_reliable": true})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, extra})
	endpoints.Main = main

	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	require.Len(t, dest.Senders, 2)
	assert.Equal(t, SenderID(0), dest.Senders[0].ID)
	assert.Equal(t, "main.example:443", dest.Senders[0].Address)
	assert.Equal(t, SenderID(1), dest.Senders[1].ID)
	assert.Equal(t, "extra.example:443", dest.Senders[1].Address)
	assert.Equal(t, Reliable, dest.Senders[0].Class)
	assert.Equal(t, Reliable, dest.Senders[1].Class)
}

func TestUnreliableExtra(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443, "is_reliable": true})
	extra := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "extra.example", "port": 443, "is_reliable": false})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, extra})
	endpoints.Main = main

	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	require.Len(t, dest.Senders, 2)
	assert.Equal(t, Reliable, dest.Senders[0].Class)
	assert.Equal(t, Unreliable, dest.Senders[1].Class)
}

func TestMRFOmitted(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443, "is_reliable": true})
	mrf := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "mrf.example", "port": 443, "is_reliable": true})
	mrf.IsMRF = true
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, mrf})
	endpoints.Main = main

	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	require.Len(t, dest.Senders, 1)
	assert.Equal(t, "main.example:443", dest.Senders[0].Address)
	assert.Equal(t, 1, dest.SkippedMRF)
}

func TestWindowingRejected(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 4)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
	extra := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "extra.example", "port": 443})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, extra})
	endpoints.Main = main

	_, err := BuildDestinationConfig(cfg, endpoints)
	require.Error(t, err)
	var window ErrWindowing
	require.ErrorAs(t, err, &window)
	assert.Equal(t, 2, window.Senders)
	assert.Equal(t, 8, window.PipelineDepth)
	assert.Equal(t, 4, window.MaxInflight)
}

func TestDDURLOverridesMainOnly(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.foldspace.max_inflight_payloads", 32)
	cfg.SetInTest("logs_config.foldspace.pipeline_depth", 8)
	cfg.SetInTest("logs_config.foldspace.dd_url", "foldspace.internal:9999")

	main := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "main.example", "port": 443})
	extra := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "extra.example", "port": 443})
	endpoints := config.NewMockEndpoints([]config.Endpoint{main, extra})
	endpoints.Main = main

	dest, err := BuildDestinationConfig(cfg, endpoints)
	require.NoError(t, err)
	assert.Equal(t, "foldspace.internal:9999", dest.Senders[0].Address)
	assert.Equal(t, "extra.example:443", dest.Senders[1].Address)
}
