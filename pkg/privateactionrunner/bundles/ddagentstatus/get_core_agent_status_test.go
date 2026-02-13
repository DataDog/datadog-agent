// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package com_datadoghq_ddagent_status

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTask(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID: "com.datadoghq.ddagent.status",
		Name:     "getCoreAgentStatus",
		Inputs:   inputs,
	}
	return task
}

func TestGetCoreAgentStatus_Success(t *testing.T) {
	ipcMock := ipcmock.New(t)

	expectedStatus := map[string]interface{}{
		"runnerStatus": "running",
		"version":      "7.50.0",
	}

	server := ipcMock.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/agent/status", r.URL.Path)
		assert.Equal(t, "json", r.URL.Query().Get("format"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedStatus)
	}))
	_ = server

	handler := NewGetCoreAgentStatusHandler(ipcMock.GetClient())
	task := newTask(map[string]interface{}{})

	result, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	status, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "running", status["runnerStatus"])
	assert.Equal(t, "7.50.0", status["version"])
}

func TestGetCoreAgentStatus_WithSection(t *testing.T) {
	ipcMock := ipcmock.New(t)

	expectedStatus := map[string]interface{}{
		"collectorStatus": "ok",
	}

	server := ipcMock.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/agent/collector/status", r.URL.Path)
		assert.Equal(t, "json", r.URL.Query().Get("format"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedStatus)
	}))
	_ = server

	handler := NewGetCoreAgentStatusHandler(ipcMock.GetClient())
	task := newTask(map[string]interface{}{
		"section": "collector",
	})

	result, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	status, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ok", status["collectorStatus"])
}

func TestGetCoreAgentStatus_WithVerbose(t *testing.T) {
	ipcMock := ipcmock.New(t)

	expectedStatus := map[string]interface{}{
		"verbose": true,
	}

	server := ipcMock.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/agent/status", r.URL.Path)
		assert.Equal(t, "json", r.URL.Query().Get("format"))
		assert.Equal(t, "true", r.URL.Query().Get("verbose"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedStatus)
	}))
	_ = server

	handler := NewGetCoreAgentStatusHandler(ipcMock.GetClient())
	task := newTask(map[string]interface{}{
		"verbose": true,
	})

	result, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	status, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, status["verbose"])
}

func TestGetCoreAgentStatus_NilClient(t *testing.T) {
	handler := NewGetCoreAgentStatusHandler(nil)
	task := newTask(map[string]interface{}{})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IPC client is not available")
}

func TestGetCoreAgentStatus_ServerError(t *testing.T) {
	ipcMock := ipcmock.New(t)

	server := ipcMock.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	_ = server

	handler := NewGetCoreAgentStatusHandler(ipcMock.GetClient())
	task := newTask(map[string]interface{}{})

	_, err := handler.Run(context.Background(), task, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get agent status")
}

func TestNewAgentStatusBundle(t *testing.T) {
	ipcMock := ipcmock.New(t)

	bundle := NewAgentStatus(ipcMock.GetClient())

	assert.NotNil(t, bundle.GetAction("getCoreAgentStatus"))
	assert.Nil(t, bundle.GetAction("nonexistent"))
}
