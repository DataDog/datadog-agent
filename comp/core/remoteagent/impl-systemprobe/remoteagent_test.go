// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package systemprobeimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestGetFlareFiles_ContainsExpectedFiles(t *testing.T) {
	impl := &remoteagentImpl{cfg: config.NewMock(t), telemetry: telemetryimpl.NewMock(t)}

	resp, err := impl.GetFlareFiles(context.Background(), &pbcore.GetFlareFilesRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Contains(t, resp.Files, "system_probe_stats.json")
	assert.Contains(t, resp.Files, "system_probe_runtime_config_dump.json")
	assert.Contains(t, resp.Files, "system_probe_telemetry.log")
}
