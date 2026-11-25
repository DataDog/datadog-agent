// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

func TestPathPrefix(t *testing.T) {
	const prefix = "3f2c9a1e-5b7d-4c1a-9e8f-0a1b2c3d4e5f"

	fi := NewServer(WithPathPrefix("/" + prefix + "/"))
	ts := httptest.NewServer(fi.server.Handler)
	defer ts.Close()

	post := func(t *testing.T, path string) int {
		t.Helper()
		resp, err := http.Post(ts.URL+path, "text/plain", strings.NewReader("totoro|5|tag:valid"))
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}
	get := func(t *testing.T, path string) (int, []byte) {
		t.Helper()
		resp, err := http.Get(ts.URL + path)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, body
	}
	payloadCount := func(t *testing.T, route string) int {
		t.Helper()
		code, body := get(t, "/fakeintake/payloads?endpoint="+route)
		require.Equal(t, http.StatusOK, code)
		var payloads api.APIFakeIntakePayloadsRawGETResponse
		require.NoError(t, json.Unmarshal(body, &payloads))
		return len(payloads.Payloads)
	}

	t.Run("stores intake requests under the prefix without it", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, post(t, "/"+prefix+"/totoro"))
		assert.Equal(t, 1, payloadCount(t, "/totoro"))
	})

	t.Run("rejects intake requests outside the prefix", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, post(t, "/kiki"))
		assert.Equal(t, http.StatusNotFound, post(t, "/0a1b2c3d-old-prefix/kiki"))
		assert.Equal(t, http.StatusNotFound, post(t, "/"+prefix+"-suffix/kiki"))
		assert.Equal(t, 0, payloadCount(t, "/kiki"))
		assert.Equal(t, 0, payloadCount(t, "/0a1b2c3d-old-prefix/kiki"))
	})

	t.Run("accepts process and orchestrator routes at the root", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, post(t, "/api/v1/collector"))
		assert.Equal(t, http.StatusOK, post(t, "/api/v2/orch"))
		assert.Equal(t, 1, payloadCount(t, "/api/v1/collector"))
	})

	t.Run("serves control endpoints at the root and under the prefix", func(t *testing.T) {
		for _, path := range []string{"/fakeintake/health", "/" + prefix + "/fakeintake/health", "/metrics"} {
			code, _ := get(t, path)
			assert.Equal(t, http.StatusOK, code, path)
		}
	})

	t.Run("counts rejected requests", func(t *testing.T) {
		_, body := get(t, "/metrics")
		assert.Contains(t, string(body), "rejected_path_prefix_requests_total 3")
	})
}

func TestNoPathPrefixAcceptsAnyRoute(t *testing.T) {
	fi := NewServer()
	ts := httptest.NewServer(fi.server.Handler)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/any/route", "text/plain", strings.NewReader("totoro"))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
