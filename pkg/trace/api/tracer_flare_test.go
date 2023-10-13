// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracerFlareProxyHandler(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "POST" {
			w.WriteHeader(404)
		}

		switch req.URL.Path {
		case "/api/ui/support/serverless/flare":
			assert.Equal(t, "test", req.Header.Get("DD-API-KEY"), "got invalid API key: %q", req.Header.Get("DD-API-KEY"))
			body, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.Equal(t, "body", string(body), "invalid request body: %q", body)
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	rec := httptest.NewRecorder()
	receiver := newTestReceiverFromConfig(newTestReceiverConfig())
	req, err := http.NewRequest("POST", srv.URL, strings.NewReader("body"))
	assert.NoError(t, err)
	receiver.tracerFlareHandler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}
