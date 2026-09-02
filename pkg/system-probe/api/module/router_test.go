// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package module

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRouterUnregister(t *testing.T) {
	mux := http.NewServeMux()
	r := NewRouter("test", mux)
	r.HandleFunc("GET /asdf", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test/asdf", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	resp := w.Result()
	_, _ = io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	r.Unregister()
	req = httptest.NewRequest("GET", "/test/asdf", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	resp = w.Result()
	_, _ = io.ReadAll(resp.Body)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
