// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package run

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/lite"
	"github.com/stretchr/testify/require"
)

func TestStartupFailureOnly(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("DD_API_KEY", "dummy")
	t.Setenv("DD_DD_URL", server.URL)
	t.Setenv("DD_HEALTH_PLATFORM_ENABLED", "true")
	t.Setenv("DD_PROXY_HTTP", "")
	p := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(p, []byte("logs_config: [broken\n"), 0600))
	original := errors.New("original startup error")
	state := startupState{params: lite.Params{ConfigPath: p}}
	require.Same(t, original, state.finish(original), "reporting failure must preserve the exact startup error")
	require.EqualValues(t, 1, calls.Load())
	require.NoError(t, state.finish(nil))
	state.started = true
	require.Same(t, original, state.finish(original))
	require.EqualValues(t, 1, calls.Load(), "after-start and nil errors must not report")
}
