// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemprobe

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTextUsesSuppliedStats(t *testing.T) {
	stats := map[string]interface{}{
		"uptime":     "42s",
		"updated_at": float64(1),
		"process":    map[string]interface{}{"running": true},
	}

	var output bytes.Buffer
	require.NoError(t, RenderText(stats, &output))

	assert.Contains(t, output.String(), "Status: Running")
	assert.Contains(t, output.String(), "Uptime: 42s")
	assert.Contains(t, output.String(), "Process")
}
