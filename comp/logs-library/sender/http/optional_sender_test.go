// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	httpclient "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type testOutcomeSink struct {
	success chan *message.Payload
	loss    chan *message.Payload
}

func (s *testOutcomeSink) Channel() chan *message.Payload             { return s.success }
func (s *testOutcomeSink) TerminalLossChannel() chan *message.Payload { return s.loss }

func endpointFromServer(t *testing.T, server *httptest.Server) config.Endpoint {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	ep := config.NewEndpoint("test-key", "", parsed.Hostname(), port, "", false)
	ep.BackoffBase = 0.05
	ep.BackoffFactor = 2
	ep.BackoffMax = 0.1
	return ep
}

func TestOptionalHTTPSenderDeliversAndDropsWhenSaturated(t *testing.T) {
	block := make(chan struct{})
	var received atomic.Int64
	var releaseOnce atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		received.Add(1)
		if !releaseOnce.Load() {
			<-block
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := configmock.New(t)
	destCtx := client.NewDestinationsContext()
	destCtx.Start()

	ep := endpointFromServer(t, server)
	optSender := NewOptionalHTTPSender(
		cfg,
		"us5",
		ep,
		destCtx,
		"logs-routing-us5",
		httpclient.JSONContentType,
		1, // small bounded queue to verify non-blocking drop
		1,
		secretsnoopimpl.NewComponent().Comp,
		metrics.NewNoopPipelineMonitor(""),
	)
	optSender.Start()

	// Send many payloads while the server is blocked; non-blocking offers must not hang.
	sent := 0
	for i := 0; i < 20; i++ {
		p := message.NewPayload([]*message.MessageMetadata{{}}, []byte(`[{"message":"hi"}]`), "", 18)
		select {
		case optSender.In() <- p:
			sent++
		default:
		}
	}
	require.GreaterOrEqual(t, sent, 1)

	// Unblock and verify at least one request succeeded, then stop cleanly.
	releaseOnce.Store(true)
	close(block)
	require.Eventually(t, func() bool { return received.Load() >= 1 }, 5*time.Second, 10*time.Millisecond)
	destCtx.Stop()
	optSender.Stop()
}

func TestHTTPSenderOutcomeSinkDistinguishesSuccessFromPermanentLoss(t *testing.T) {
	var statusCode atomic.Int32
	statusCode.Store(http.StatusBadRequest) // Permanent 400 first

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.WriteHeader(int(statusCode.Load()))
	}))
	defer server.Close()

	cfg := configmock.New(t)
	destCtx := client.NewDestinationsContext()
	destCtx.Start()
	defer destCtx.Stop()

	ep := endpointFromServer(t, server)
	eps := config.NewEndpointsWithBatchSettings(ep, nil, false, true, time.Second, 1, 10, 1024*1024, 16)
	sink := &testOutcomeSink{
		success: make(chan *message.Payload, 4),
		loss:    make(chan *message.Payload, 4),
	}
	s := NewHTTPSender(cfg, sink, 8, sender.NewServerlessMeta(false), eps, destCtx, "test-outcome", httpclient.JSONContentType, "", 1, 1, 1, 1, secretsnoopimpl.NewComponent().Comp, metrics.NewNoopPipelineMonitor(""))
	s.Start()
	defer s.Stop()

	// 400 permanent rejection -> must arrive on TerminalLossChannel, NOT Channel.
	p1 := message.NewPayload([]*message.MessageMetadata{{}}, []byte(`[{"message":"bad"}]`), "", 19)
	s.In() <- p1
	select {
	case <-sink.loss:
	case <-sink.success:
		t.Fatal("400 permanent rejection reported on success channel")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for terminal loss")
	}

	// 200 OK -> must arrive on Channel (success), NOT TerminalLossChannel.
	statusCode.Store(http.StatusOK)
	p2 := message.NewPayload([]*message.MessageMetadata{{}}, []byte(`[{"message":"ok"}]`), "", 18)
	s.In() <- p2
	select {
	case <-sink.success:
	case <-sink.loss:
		t.Fatal("200 OK reported on terminal loss channel")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for success")
	}
}
