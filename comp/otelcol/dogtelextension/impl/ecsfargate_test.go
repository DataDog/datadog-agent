// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package dogtelextensionimpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFetchECSTaskARN_Success verifies a well-formed /task response yields the TaskARN.
func TestFetchECSTaskARN_Success(t *testing.T) {
	const wantARN = "arn:aws:ecs:us-east-1:123456789012:task/my-cluster/abc123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/task", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"TaskARN":"` + wantARN + `","Cluster":"my-cluster"}`))
	}))
	defer server.Close()

	t.Setenv(ecsMetadataURIv4EnvVar, server.URL)

	arn, err := fetchECSTaskARN(context.Background())
	require.NoError(t, err)
	assert.Equal(t, wantARN, arn)
}

// TestFetchECSTaskARN_EnvVarNotSet verifies an error is returned when the metadata
// endpoint env var is absent, as happens off of ECS Fargate.
func TestFetchECSTaskARN_EnvVarNotSet(t *testing.T) {
	// An empty value is indistinguishable from unset via os.Getenv, and is how
	// the env looks off of ECS Fargate.
	t.Setenv(ecsMetadataURIv4EnvVar, "")

	_, err := fetchECSTaskARN(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), ecsMetadataURIv4EnvVar)
}

// TestFetchECSTaskARN_HTTPError verifies a non-200 response is treated as an error.
func TestFetchECSTaskARN_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	t.Setenv(ecsMetadataURIv4EnvVar, server.URL)

	_, err := fetchECSTaskARN(context.Background())
	require.Error(t, err)
}

// TestFetchECSTaskARN_MalformedJSON verifies decode errors are surfaced.
func TestFetchECSTaskARN_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	t.Setenv(ecsMetadataURIv4EnvVar, server.URL)

	_, err := fetchECSTaskARN(context.Background())
	require.Error(t, err)
}

// TestFetchECSTaskARN_EmptyTaskARN verifies a response missing TaskARN is an error,
// rather than silently returning an empty tag value.
func TestFetchECSTaskARN_EmptyTaskARN(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Cluster":"my-cluster"}`))
	}))
	defer server.Close()

	t.Setenv(ecsMetadataURIv4EnvVar, server.URL)

	_, err := fetchECSTaskARN(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TaskARN")
}

// TestFetchECSTaskARN_ConnectionRefused verifies unreachable endpoints surface an error
// instead of hanging, exercising the request-level (as opposed to decode-level) failure path.
func TestFetchECSTaskARN_ConnectionRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	addr := server.URL
	server.Close() // closed immediately: addr is now guaranteed unreachable

	t.Setenv(ecsMetadataURIv4EnvVar, addr)

	_, err := fetchECSTaskARN(context.Background())
	require.Error(t, err)
}
