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

	require.Greater(t, len(cfg.EbpfCollectorConfig.Tracers), 0)
	require.NotContains(t, cfg.EbpfCollectorConfig.Tracers, "go")
	require.NotContains(t, cfg.EbpfCollectorConfig.Tracers, "labels")

	cfg.ReporterConfig.CollectContext = false
	require.NoError(t, cfg.Validate())
	require.NotContains(t, cfg.EbpfCollectorConfig.Tracers, "labels")

	cfg.ReporterConfig.CollectContext = true
	require.NoError(t, cfg.Validate())
	require.Contains(t, cfg.EbpfCollectorConfig.Tracers, "labels")
}

func TestServiceNameEnvVars(t *testing.T) {
	config := defaultConfig()
	cfg := config.(Config)
	cfg.EnableSplitByService = false
	require.NoError(t, cfg.Validate())
	require.Equal(t, "", cfg.EbpfCollectorConfig.IncludeEnvVars)

	cfg.EnableSplitByService = true
	require.NoError(t, cfg.Validate())
	require.Equal(t, strings.Join(reporter.ServiceNameEnvVars, ","), cfg.EbpfCollectorConfig.IncludeEnvVars)
}
