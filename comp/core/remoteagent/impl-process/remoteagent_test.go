// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package processimpl

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestGetStatusDetails(t *testing.T) {
	impl := &remoteagentImpl{cfg: config.NewMock(t), telemetry: telemetryimpl.NewMock(t)}

	resp, err := impl.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.MainSection)
	assert.NotNil(t, resp.MainSection.Fields)
	assert.Nil(t, resp.NamedSections)
}

func TestGetFlareFiles_ContainsExpectedFiles(t *testing.T) {
	impl := &remoteagentImpl{cfg: config.NewMock(t), telemetry: telemetryimpl.NewMock(t)}

	resp, err := impl.GetFlareFiles(context.Background(), &pbcore.GetFlareFilesRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Contains(t, resp.Files, "process_agent_status.json")
	assert.Contains(t, resp.Files, "process_agent_runtime_config_dump.json")
}

func TestGetFlareFiles_FilesAreValidJSON(t *testing.T) {
	impl := &remoteagentImpl{cfg: config.NewMock(t), telemetry: telemetryimpl.NewMock(t)}

	resp, err := impl.GetFlareFiles(context.Background(), &pbcore.GetFlareFilesRequest{})
	require.NoError(t, err)

	var statusData map[string]any
	require.NoError(t, json.Unmarshal(resp.Files["process_agent_status.json"], &statusData),
		"process_agent_status.json should be valid JSON")

	var configData map[string]any
	require.NoError(t, json.Unmarshal(resp.Files["process_agent_runtime_config_dump.json"], &configData),
		"process_agent_runtime_config_dump.json should be valid JSON")
}
