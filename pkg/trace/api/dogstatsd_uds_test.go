// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build !windows

package api

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/require"
)

func TestDogStatsDReverseProxyEndToEndUDS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	sock := "/tmp/com.datadoghq.datadog-agent.trace-agent.dogstatsd.test.sock"

	cfg := config.New()
	cfg.StatsdSocket = sock            // this should get ignored
	cfg.StatsdHost = "this is invalid" // this should get ignored
	cfg.StatsdPort = 0                 // this should trigger 503
	receiver := newTestReceiverFromConfig(cfg)
	proxy := receiver.dogstatsdProxyHandler()
	require.NotNil(t, proxy)
	rec := httptest.NewRecorder()

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	server := httptest.NewUnstartedServer(proxy)
	server.Listener = l
	server.Start()
	defer server.Close()

	// Test metrics
	body := io.NopCloser(bytes.NewBufferString("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))
	proxy.ServeHTTP(rec, httptest.NewRequest("POST", "/", body))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
