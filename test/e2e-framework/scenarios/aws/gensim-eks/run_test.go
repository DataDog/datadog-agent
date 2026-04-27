// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gensimeks

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderAgentValues_LogsEnabled(t *testing.T) {
	image := "docker.io/datadog/agent-dev:my-tag"
	mode := "record-parquet"

	out, err := renderAgentValues(image, mode, true, true)
	require.NoError(t, err)
	require.Contains(t, out, "logs:")
	require.Contains(t, out, "enabled: true")
	require.Contains(t, out, "containerCollectAll: true")

	out, err = renderAgentValues(image, mode, false, true)
	require.NoError(t, err)
	require.Contains(t, out, "enabled: false")
	require.NotContains(t, out, "containerCollectAll")
}

func TestRenderAgentValues_MetricsIngestion(t *testing.T) {
	image := "docker.io/datadog/agent-dev:my-tag"
	mode := "live-anomaly-detection"

	// Default: metric ingestion stays enabled, no override emitted.
	out, err := renderAgentValues(image, mode, true, true)
	require.NoError(t, err)
	require.NotContains(t, out, "DD_OBSERVER_INGEST_METRICS_ENABLED")
	require.NotContains(t, out, "ingest_metrics:")

	// Disabled: emit both the env var and the customAgentConfig override
	// so the observer drops external metrics at the handle factory.
	out, err = renderAgentValues(image, mode, true, false)
	require.NoError(t, err)
	require.Contains(t, out, "DD_OBSERVER_INGEST_METRICS_ENABLED")
	require.Contains(t, out, `value: "false"`)
	require.Contains(t, out, "ingest_metrics:")
	require.Contains(t, out, "enabled: false")
}

func TestRenderAgentValues_InvalidImage(t *testing.T) {
	_, err := renderAgentValues("notag", "record-parquet", true, true)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "invalid image reference"))
}
