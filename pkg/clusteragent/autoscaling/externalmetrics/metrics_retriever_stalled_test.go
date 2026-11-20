// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package externalmetrics

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/externalmetrics/model"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

// memNetwork is an in-memory network: connections are net.Pipe pairs, which block on channels and
// timers. Unlike a loopback TCP socket, blocking on them is durable, so inside a synctest bubble the
// fake clock keeps advancing while a request hangs.
type memNetwork struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newMemNetwork() *memNetwork {
	return &memNetwork{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (n *memNetwork) Accept() (net.Conn, error) {
	select {
	case conn := <-n.conns:
		return conn, nil
	case <-n.done:
		return nil, net.ErrClosed
	}
}

func (n *memNetwork) Close() error {
	n.once.Do(func() { close(n.done) })
	return nil
}

func (n *memNetwork) Addr() net.Addr { return memAddr{} }

// dial ignores the requested address, there is a single server.
func (n *memNetwork) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case n.conns <- server:
		return client, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type memAddr struct{}

func (memAddr) Network() string { return "mem" }
func (memAddr) String() string  { return "mem" }

// timeoutRequestsCount returns the number of requests to Datadog that timed out, from the
// datadog_requests counter.
func timeoutRequestsCount(t *testing.T) float64 {
	t.Helper()

	families, err := telemetryimpl.GetCompatComponent().Gather(telemetry.NoFilter)
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() != "datadog_requests" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "status" && label.GetValue() == "timeout" {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// TestRetrieveMetricsStalledRequest reproduces
// https://github.com/DataDog/datadog-agent/issues/60529: a request to Datadog that never completes
// (for instance after a connectivity issue) must not prevent the other metrics from being refreshed.
//
// It runs the real MetricsRetriever, Processor and Datadog client against a real HTTP server whose
// handler hangs for one of the queries, in a synctest bubble: the hang and the timeouts only take
// virtual time.
func TestRetrieveMetricsStalledRequest(t *testing.T) {
	const (
		stalledQuery = "avg:stalled{*}"
		healthyQuery = "avg:healthy{*}"
		// Longer than any sensible timeout, as the outage in the report lasted for minutes.
		hangLimit = 10 * time.Minute
	)

	cfg := configmock.New(t)
	// One query per request, so that each query is sent on its own.
	cfg.Set("external_metrics_provider.chunk_size", 1, pkgconfigmodel.SourceLocalConfigProcess)

	synctest.Test(t, func(t *testing.T) {
		network := newMemNetwork()
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			query := r.URL.Query().Get("query")
			if query == stalledQuery {
				// Never answers, until the test is over.
				<-t.Context().Done()
				return
			}

			_ = json.NewEncoder(w).Encode(map[string]any{"series": []datadog.Series{{
				Metric:     new("healthy"),
				Scope:      new("*"),
				Expression: new(query),
				QueryIndex: new(0),
				Points:     []datadog.DataPoint{{new(float64(time.Now().Add(-30 * time.Second).UnixMilli())), new(float64(42))}},
			}}})
		})}
		go func() { _ = server.Serve(network) }()

		transport := &http.Transport{DialContext: network.dial}
		defer func() {
			transport.CloseIdleConnections()
			_ = server.Close()
			_ = network.Close()
		}()

		client := datadog.NewClient("api-key", "app-key")
		client.SetBaseUrl("http://datadog.test")
		// The timeout of the Datadog clients created by the Agent.
		client.HttpClient = &http.Client{
			Transport: transport,
			Timeout:   time.Duration(cfg.GetInt("external_metrics_provider.endpoint_timeout")) * time.Second,
		}
		client.RetryTimeout = 3 * time.Second

		store := NewDatadogMetricsInternalStore()
		for _, id := range []string{"stalled", "healthy"} {
			ddm := model.DatadogMetricInternal{ID: id, Active: true}
			if id == "stalled" {
				ddm.SetQueries(stalledQuery)
			} else {
				ddm.SetQueries(healthyQuery)
			}
			store.Set(id, ddm, "utest")
		}

		retriever, err := NewMetricsRetriever(0, 120, autoscalers.NewProcessor(client), getIsLeaderFunction(true), &store, false)
		require.NoError(t, err)

		timeoutsBefore := timeoutRequestsCount(t)

		go retriever.retrieveMetricsValues()

		// Time only advances once every goroutine is blocked, so this takes no real time.
		time.Sleep(hangLimit)
		synctest.Wait()

		assert.Equal(t, timeoutsBefore+1, timeoutRequestsCount(t), "the timed out request should be counted")

		healthy := store.Get("healthy")
		require.NotNil(t, healthy)
		assert.True(t, healthy.Valid, "the healthy metric should be refreshed after %s, a single stalled request must not hold up the others (error: %v)", hangLimit, healthy.Error)
		assert.Equal(t, float64(42), healthy.Value)
		assert.NoError(t, healthy.Error)

		stalled := store.Get("stalled")
		require.NotNil(t, stalled)
		assert.False(t, stalled.Valid, "the stalled metric should be reported as failed")
		assert.Error(t, stalled.Error)
	})
}
