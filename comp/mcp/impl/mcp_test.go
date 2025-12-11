// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package mcpimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	mcpcomp "github.com/DataDog/datadog-agent/comp/mcp/def"
	"github.com/DataDog/datadog-agent/pkg/mcp/server"
	"github.com/DataDog/datadog-agent/pkg/mcp/types"
)

func TestNewComponentDisabled(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("mcp.enabled", false)

	requires := Requires{
		Lc:     lc,
		Config: cfg,
		Params: mcpcomp.NewParams(),
	}

	provides, err := NewComponent(requires)
	require.NoError(t, err)
	require.NotNil(t, provides.Comp)

	assert.False(t, provides.Comp.IsEnabled())
	assert.Nil(t, provides.Comp.ListTools())

	// Lifecycle hooks should still be registered
	lc.AssertHooksNumber(1)

	// Start and stop should work without error when disabled
	ctx := context.Background()
	require.NoError(t, lc.Start(ctx))
	require.NoError(t, lc.Stop(ctx))
}

func TestNewComponentEnabledByConfig(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("mcp.enabled", true)
	cfg.SetWithoutSource("mcp.server.address", "unix:///tmp/mcp-test.sock")
	cfg.SetWithoutSource("mcp.tools.process.enabled", true)

	requires := Requires{
		Lc:     lc,
		Config: cfg,
		Params: mcpcomp.NewParams(),
	}

	provides, err := NewComponent(requires)
	require.NoError(t, err)
	require.NotNil(t, provides.Comp)

	assert.True(t, provides.Comp.IsEnabled())

	// Should have registered the GetProcessSnapshot tool
	tools := provides.Comp.ListTools()
	assert.Contains(t, tools, "GetProcessSnapshot")
}

func TestNewComponentDisabledByParams(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("mcp.enabled", true) // Config enables it

	requires := Requires{
		Lc:     lc,
		Config: cfg,
		Params: mcpcomp.NewDisabledParams(), // But params disable it
	}

	provides, err := NewComponent(requires)
	require.NoError(t, err)
	require.NotNil(t, provides.Comp)

	// Should be disabled because params override config
	assert.False(t, provides.Comp.IsEnabled())
}

func TestHandleToolCallDisabled(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("mcp.enabled", false)

	requires := Requires{
		Lc:     lc,
		Config: cfg,
		Params: mcpcomp.NewParams(),
	}

	provides, err := NewComponent(requires)
	require.NoError(t, err)

	req := &server.ToolRequest{
		ToolName:  "GetProcessSnapshot",
		RequestID: "test-123",
	}

	_, err = provides.Comp.HandleToolCall(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

func TestHandleToolCallNotStarted(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	cfg := config.NewMock(t)
	cfg.SetWithoutSource("mcp.enabled", true)
	cfg.SetWithoutSource("mcp.server.address", "unix:///tmp/mcp-test.sock")
	cfg.SetWithoutSource("mcp.tools.process.enabled", true)

	requires := Requires{
		Lc:     lc,
		Config: cfg,
		Params: mcpcomp.NewParams(),
	}

	provides, err := NewComponent(requires)
	require.NoError(t, err)

	// Don't start the component - try to handle a request
	req := &server.ToolRequest{
		ToolName:  "GetProcessSnapshot",
		RequestID: "test-123",
	}

	_, err = provides.Comp.HandleToolCall(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not started")
}

func TestTelemetryHandler(t *testing.T) {
	tel := newMCPTelemetry()

	// Create a mock handler
	mockHandler := &mockToolHandler{
		response: &types.ToolResponse{
			ToolName:  "test-tool",
			Result:    "success",
			RequestID: "req-1",
		},
	}

	// Wrap it with telemetry
	wrappedHandler := &telemetryHandler{
		wrapped:   mockHandler,
		telemetry: tel,
	}

	req := &types.ToolRequest{
		ToolName:  "test-tool",
		RequestID: "req-1",
	}

	// Make a successful request
	resp, err := wrappedHandler.Handle(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "success", resp.Result)

	// Verify telemetry was recorded
	assert.Equal(t, int64(1), tel.GetRequestCount("test-tool"))
	assert.Equal(t, int64(0), tel.GetErrorCount("test-tool"))

	// Make an error request
	mockHandler.shouldError = true
	_, err = wrappedHandler.Handle(context.Background(), req)
	assert.Error(t, err)

	// Verify error was tracked
	assert.Equal(t, int64(2), tel.GetRequestCount("test-tool"))
	assert.Equal(t, int64(1), tel.GetErrorCount("test-tool"))
}

func TestTelemetryHandlerLatency(t *testing.T) {
	tel := newMCPTelemetry()

	// Create a handler with artificial delay
	mockHandler := &mockToolHandler{
		response: &types.ToolResponse{ToolName: "slow-tool"},
		delay:    50 * time.Millisecond,
	}

	wrappedHandler := &telemetryHandler{
		wrapped:   mockHandler,
		telemetry: tel,
	}

	req := &types.ToolRequest{ToolName: "slow-tool"}

	_, err := wrappedHandler.Handle(context.Background(), req)
	require.NoError(t, err)

	// Latency should be recorded (at least 50ms)
	avgLatency := tel.GetAverageLatency("slow-tool")
	assert.GreaterOrEqual(t, avgLatency, 40.0) // Allow some margin
}

// mockToolHandler is a test double for server.Handler
type mockToolHandler struct {
	response    *types.ToolResponse
	shouldError bool
	delay       time.Duration
}

func (m *mockToolHandler) Handle(ctx context.Context, req *types.ToolRequest) (*types.ToolResponse, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if m.shouldError {
		return nil, assert.AnError
	}
	return m.response, nil
}
