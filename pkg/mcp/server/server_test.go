// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/mcp/types"
)

// mockHandler for testing
type mockHandler struct {
	called bool
}

func (m *mockHandler) Handle(ctx context.Context, req *types.ToolRequest) (*types.ToolResponse, error) {
	m.called = true
	return &types.ToolResponse{
		ToolName:  req.ToolName,
		Result:    "success",
		RequestID: req.RequestID,
	}, nil
}

func TestNewMCPServer(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", false)

	srv, err := NewMCPServer(cfg)
	require.NoError(t, err)
	require.NotNil(t, srv)
	assert.False(t, srv.IsEnabled())
}

func TestNewMCPServerEnabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", true)
	cfg.SetWithoutSource("mcp.server.address", "unix:///tmp/test.sock")

	srv, err := NewMCPServer(cfg)
	require.NoError(t, err)
	require.NotNil(t, srv)
	assert.True(t, srv.IsEnabled())
}

func TestRegisterTool(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	srv, err := NewMCPServer(cfg)
	require.NoError(t, err)

	handler := &mockHandler{}
	err = srv.RegisterTool("test-tool", handler)
	require.NoError(t, err)

	tools := srv.ListTools()
	assert.Contains(t, tools, "test-tool")
}

func TestHandleToolCall(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	srv, err := NewMCPServer(cfg)
	require.NoError(t, err)

	handler := &mockHandler{}
	err = srv.RegisterTool("test-tool", handler)
	require.NoError(t, err)

	req := &ToolRequest{
		ToolName:   "test-tool",
		Parameters: map[string]interface{}{},
		RequestID:  "test-123",
	}

	resp, err := srv.HandleToolCall(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, handler.called)
	assert.Equal(t, "test-tool", resp.ToolName)
	assert.Equal(t, "test-123", resp.RequestID)
}

func TestHandleToolCallUnknownTool(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	srv, err := NewMCPServer(cfg)
	require.NoError(t, err)

	req := &ToolRequest{
		ToolName:   "unknown-tool",
		Parameters: map[string]interface{}{},
		RequestID:  "test-123",
	}

	resp, err := srv.HandleToolCall(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
	assert.NotEmpty(t, resp.Error)
}

func TestStartStop(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", true)

	srv, err := NewMCPServer(cfg)
	require.NoError(t, err)

	err = srv.Start(context.Background())
	require.NoError(t, err)

	// Starting again should fail
	err = srv.Start(context.Background())
	assert.Error(t, err)

	err = srv.Stop()
	require.NoError(t, err)

	// Stopping again should not fail
	err = srv.Stop()
	require.NoError(t, err)
}

func TestStartStopDisabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetTestOnlyDynamicSchema(true)
	cfg.SetWithoutSource("mcp.enabled", false)

	srv, err := NewMCPServer(cfg)
	require.NoError(t, err)

	// Should not error when disabled
	err = srv.Start(context.Background())
	require.NoError(t, err)

	err = srv.Stop()
	require.NoError(t, err)
}
