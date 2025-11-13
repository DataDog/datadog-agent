// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"strings"
	"testing"

	"github.com/DataDog/dd-otel-host-profiler/reporter"
	"github.com/stretchr/testify/require"
)

func TestTracers(t *testing.T) {
	config := defaultConfig()
	cfg := config.(Config)

	require.Greater(t, len(cfg.Ebpfcollector.Tracers), 0)
	require.NotContains(t, cfg.Ebpfcollector.Tracers, "go")
	require.NotContains(t, cfg.Ebpfcollector.Tracers, "labels")

	cfg.ReporterConfig.CollectContext = false
	cfg.SymbolUploader.Enabled = false
	require.NoError(t, cfg.Validate())
	require.NotContains(t, cfg.Ebpfcollector.Tracers, "labels")

	cfg.ReporterConfig.CollectContext = true
	require.NoError(t, cfg.Validate())
	require.Contains(t, cfg.Ebpfcollector.Tracers, "labels")
}

func TestServiceNameEnvVars(t *testing.T) {
	config := defaultConfig()
	cfg := config.(Config)
	cfg.EnableSplitByService = false
	cfg.SymbolUploader.Enabled = false
	require.NoError(t, cfg.Validate())
	require.Equal(t, "", cfg.Ebpfcollector.IncludeEnvVars)

	cfg.EnableSplitByService = true
	require.NoError(t, cfg.Validate())
	require.Equal(t, strings.Join(reporter.ServiceNameEnvVars, ","), cfg.Ebpfcollector.IncludeEnvVars)
}

func TestSymbolUploader(t *testing.T) {
	config := defaultConfig()
	cfg := config.(Config)
	cfg.SymbolUploader.Enabled = false
	require.NoError(t, cfg.Validate())

	cfg.SymbolUploader.Enabled = true
	require.Error(t, errSymbolEndpointsRequired, cfg.Validate())

	cfg.SymbolUploader.SymbolEndpoints = []reporter.SymbolEndpoint{{}}
	require.Error(t, errSymbolEndpointsSiteRequired, cfg.Validate())
	cfg.SymbolUploader.SymbolEndpoints[0].Site = "datadoghq.com"
	require.Error(t, errSymbolEndpointsAPIKeyRequired, cfg.Validate())
	cfg.SymbolUploader.SymbolEndpoints[0].APIKey = "1234567890"
	require.Error(t, errSymbolEndpointsAppKeyRequired, cfg.Validate())
	cfg.SymbolUploader.SymbolEndpoints[0].AppKey = "1234567890"
	require.NoError(t, cfg.Validate())
}
