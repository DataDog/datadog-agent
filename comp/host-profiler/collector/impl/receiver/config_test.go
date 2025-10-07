// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package receiver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTracers(t *testing.T) {
	config := defaultConfig()
	cfg := config.(Config)

	require.Greater(t, len(cfg.Tracers), 0)
	require.NotContains(t, cfg.Tracers, "go")
	require.NotContains(t, cfg.Tracers, "labels")

	cfg.ReporterConfig.CollectContext = false
	require.NoError(t, cfg.Validate())
	require.NotContains(t, cfg.Tracers, "labels")

	cfg.ReporterConfig.CollectContext = true
	require.NoError(t, cfg.Validate())
	require.Contains(t, cfg.Tracers, "labels")
}
