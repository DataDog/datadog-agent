// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/require"
)

func TestPayloadPageHTTP(t *testing.T) {
	fi := NewServer()
	require.NoError(t, fi.store.AppendPayload("logs", "key", []byte("body"), "identity", "json", time.Now()))
	request := func(method, target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		fi.server.Handler.ServeHTTP(w, httptest.NewRequest(method, target, nil))
		return w
	}
	for _, target := range []string{"/fakeintake/payloads/page", "/fakeintake/payloads/page?endpoint=logs&limit=0", "/fakeintake/payloads/page?endpoint=logs&limit=129", "/fakeintake/payloads/page?endpoint=logs&limit=1&cursor=bad"} {
		require.Equal(t, 400, request("GET", target).Code)
	}
	require.Equal(t, http.StatusMethodNotAllowed, request("POST", "/fakeintake/payloads/page?endpoint=logs&limit=1").Code)
	w := request("GET", "/fakeintake/payloads/page?endpoint=logs&limit=1")
	require.Equal(t, 200, w.Code)
	var page api.PayloadPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Payloads, 1)
	require.Equal(t, []byte("body"), page.Payloads[0].Data)
	fi.store.Flush()
	require.Equal(t, http.StatusGone, request("GET", "/fakeintake/payloads/page?endpoint=logs&limit=1&cursor="+page.NextCursor).Code)
}
