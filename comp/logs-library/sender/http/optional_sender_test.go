// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	httpclient "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestOptionalHTTPSenderNonBlockingDropAndDelivery(t *testing.T) {
	var delivered atomic.Int32
	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
			return
		}
		delivered.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)

	ep := config.NewMockEndpointWithOptions(map[string]any{
		"api_key":     "opt-key",
		"host":        parsed.Hostname(),
		"port":        port,
		"use_ssl":     false,
		"is_reliable": false,
	})

	destCtx := client.NewDestinationsContext()
	destCtx.Start()
	defer destCtx.Stop()

	cfg := configmock.New(t)
	s := NewOptionalHTTPSender(cfg, "us5", ep, destCtx, "test-opt", httpclient.JSONContentType, 1, 1, secretsnoopimpl.NewComponent().Comp, metrics.NewNoopPipelineMonitor(""))
	s.Start()

	// Fill bounded queue while server blocks; TrySend must return immediately and drop excess.
	dropped := 0
	for i := 0; i < 10; i++ {
		p := message.NewPayload([]*message.MessageMetadata{{}}, []byte(`[{"message":"x"}]`), "", 1)
		if !s.TrySend(p) {
			dropped++
		}
	}
	assert.Greater(t, dropped, 0, "bounded optional sender must drop without blocking when saturated")

	close(block)
	require.Eventually(t, func() bool {
		return delivered.Load() >= 1
	}, 5*time.Second, 10*time.Millisecond)

	s.Stop()
}
