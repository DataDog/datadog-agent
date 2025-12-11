// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/mcp/types"
)

// mockHandler is a simple handler for testing
type mockHandler struct {
	name string
}

func (m *mockHandler) Handle(ctx context.Context, req *types.ToolRequest) (*types.ToolResponse, error) {
	return &types.ToolResponse{
		ToolName:  req.ToolName,
		Result:    m.name,
		RequestID: req.RequestID,
	}, nil
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
	assert.Empty(t, r.List())
}

func TestRegister(t *testing.T) {
	r := NewRegistry()
	handler := &mockHandler{name: "test"}

	err := r.Register("test-tool", handler)
	require.NoError(t, err)

	tools := r.List()
	assert.Len(t, tools, 1)
	assert.Contains(t, tools, "test-tool")
}

func TestRegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	handler1 := &mockHandler{name: "test1"}
	handler2 := &mockHandler{name: "test2"}

	err := r.Register("test-tool", handler1)
	require.NoError(t, err)

	err = r.Register("test-tool", handler2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestGetHandler(t *testing.T) {
	r := NewRegistry()
	handler := &mockHandler{name: "test"}

	err := r.Register("test-tool", handler)
	require.NoError(t, err)

	retrieved, err := r.GetHandler("test-tool")
	require.NoError(t, err)
	assert.Equal(t, handler, retrieved)
}

func TestGetHandlerNotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.GetHandler("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUnregister(t *testing.T) {
	r := NewRegistry()
	handler := &mockHandler{name: "test"}

	err := r.Register("test-tool", handler)
	require.NoError(t, err)

	err = r.Unregister("test-tool")
	require.NoError(t, err)

	assert.Empty(t, r.List())
}

func TestUnregisterNotFound(t *testing.T) {
	r := NewRegistry()

	err := r.Unregister("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListMultiple(t *testing.T) {
	r := NewRegistry()

	handlers := map[string]*mockHandler{
		"tool1": {name: "test1"},
		"tool2": {name: "test2"},
		"tool3": {name: "test3"},
	}

	for name, handler := range handlers {
		err := r.Register(name, handler)
		require.NoError(t, err)
	}

	tools := r.List()
	assert.Len(t, tools, 3)
	for name := range handlers {
		assert.Contains(t, tools, name)
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	done := make(chan bool)

	// Register handlers concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			handler := &mockHandler{name: string(rune(id))}
			_ = r.Register(string(rune('a'+id)), handler)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	tools := r.List()
	assert.True(t, len(tools) > 0)
}
