// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package securityagentimpl

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// statusMock is a minimal status.Component implementation for testing.
type statusMock struct {
	statusJSON []byte
	err        error
}

func (m *statusMock) GetStatus(_ string, _ bool, _ ...string) ([]byte, error) {
	return m.statusJSON, m.err
}

func (m *statusMock) GetSections() []string {
	return nil
}

func (m *statusMock) GetStatusBySections(_ []string, _ string, _ bool) ([]byte, error) {
	return m.statusJSON, m.err
}

var _ status.Component = (*statusMock)(nil)

func newStatusMock(statusJSON []byte) *statusMock {
	return &statusMock{statusJSON: statusJSON}
}

func TestGetStatusDetails_IncludesExpvarAndStatus(t *testing.T) {
	statusJSON := []byte(`{"agent":"security-agent","status":"running"}`)
	impl := &remoteagentImpl{
		cfg:        config.NewMock(t),
		statusComp: newStatusMock(statusJSON),
	}

	resp, err := impl.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.MainSection)
	assert.NotNil(t, resp.MainSection.Fields)
	// Status JSON from the status component should be present
	assert.Equal(t, string(statusJSON), resp.MainSection.Fields["status"])
}

func TestGetFlareFiles_ContainsExpectedFiles(t *testing.T) {
	statusJSON := []byte(`{"agent":"security-agent"}`)
	impl := &remoteagentImpl{
		cfg:        config.NewMock(t),
		statusComp: newStatusMock(statusJSON),
	}

	resp, err := impl.GetFlareFiles(context.Background(), &pbcore.GetFlareFilesRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Contains(t, resp.Files, "security_agent_status.json")
	assert.Contains(t, resp.Files, "security_agent_expvar_dump.json")
	assert.Contains(t, resp.Files, "security_agent_runtime_config_dump.json")
}

func TestGetFlareFiles_StatusFileMatchesStatusComponent(t *testing.T) {
	statusJSON := []byte(`{"agent":"security-agent","status":"running"}`)
	impl := &remoteagentImpl{
		cfg:        config.NewMock(t),
		statusComp: newStatusMock(statusJSON),
	}

	resp, err := impl.GetFlareFiles(context.Background(), &pbcore.GetFlareFilesRequest{})
	require.NoError(t, err)

	assert.Equal(t, statusJSON, resp.Files["security_agent_status.json"])
}

func TestGetFlareFiles_FilesAreValidJSON(t *testing.T) {
	impl := &remoteagentImpl{
		cfg:        config.NewMock(t),
		statusComp: newStatusMock([]byte(`{}`)),
	}

	resp, err := impl.GetFlareFiles(context.Background(), &pbcore.GetFlareFilesRequest{})
	require.NoError(t, err)

	var expvarData map[string]any
	require.NoError(t, json.Unmarshal(resp.Files["security_agent_expvar_dump.json"], &expvarData),
		"security_agent_expvar_dump.json should be valid JSON")

	var configData map[string]any
	require.NoError(t, json.Unmarshal(resp.Files["security_agent_runtime_config_dump.json"], &configData),
		"security_agent_runtime_config_dump.json should be valid JSON")
}
