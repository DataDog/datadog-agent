// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && test

package lite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/stretchr/testify/require"
)

func TestRescueResolvesOnlyReportingSecrets(t *testing.T) {
	cleanEnv(t)
	requestFile := filepath.Join(t.TempDir(), "request.json")
	command := filepath.Join(t.TempDir(), "backend")
	require.NoError(t, os.WriteFile(command, []byte("#!/bin/sh\ncat > \"$1\"\nprintf '%s' '{\"key\":{\"value\":\"resolved-sentinel\"}}'\n"), 0700))
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- r.Header.Get("DD-API-KEY") + "\n" + string(body)
	}))
	defer server.Close()
	p := Params{ConfigPath: configFile(t, fmt.Sprintf("api_key: ENC[key]\ndd_url: %s\nsecret_backend_command: %s\nsecret_backend_arguments: [%q]\nlogs_config:\n  unrelated: ENC[unavailable]\n", server.URL, command, requestFile))}
	require.NoError(t, Rescue(context.Background(), p, errors.New("startup failed: resolved-sentinel ENC[key]")))
	select {
	case request := <-requests:
		require.Contains(t, request, "resolved-sentinel\n")
		require.NotContains(t, request[len("resolved-sentinel\n"):], "sentinel")
		require.NotContains(t, request, "ENC[key]")
	default:
		t.Fatal("ENC-backed startup report was not delivered")
	}
	raw, err := os.ReadFile(requestFile)
	require.NoError(t, err)
	var payload struct {
		Version string   `json:"version"`
		Secrets []string `json:"secrets"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, secrets.PayloadVersion, payload.Version)
	require.Equal(t, []string{"key"}, payload.Secrets, "unrelated secrets must not reach the helper")
	// The real resolver must still reject unsafe executable permissions.
	require.NoError(t, os.Chmod(command, 0777))
	require.Error(t, Rescue(context.Background(), p, errors.New("failed")))
	require.Empty(t, requests)
}

func TestRescueRedactsOverriddenCredentials(t *testing.T) {
	cleanEnv(t)
	t.Setenv("DD_API_KEY", "env-sentinel")
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
	}))
	defer server.Close()
	p := Params{ConfigPath: configFile(t, "api_key: file-sentinel\ndd_url: "+server.URL+"\nsecret_backend_config:\n  token: backend-sentinel\n")}
	require.NoError(t, Rescue(context.Background(), p, errors.New("failure mentions file-sentinel env-sentinel backend-sentinel")))
	require.NotContains(t, <-received, "sentinel")
}
