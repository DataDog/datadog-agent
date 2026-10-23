// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package privateactionrunnerimpl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
)

func TestStopExecutorFlushesTracesAfterDraining(t *testing.T) {
	type span struct {
		Service  string `json:"service"`
		Name     string `json:"name"`
		TraceID  uint64 `json:"trace_id"`
		SpanID   uint64 `json:"span_id"`
		ParentID uint64 `json:"parent_id"`
		Duration int64  `json:"duration"`
		Error    int    `json:"error"`
	}
	payloads := make(chan [][]span, 10)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/apmtelemetry", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("dd-api-key"))
		var body struct {
			RequestType string `json:"request_type"`
			Payload     struct {
				Traces [][]span `json:"traces"`
			} `json:"payload"`
		}
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, "traces", body.RequestType)
		payloads <- body.Payload.Traces
	}))
	defer collector.Close()
	collectorURL, err := url.Parse(collector.URL)
	require.NoError(t, err)
	client := &http.Client{Transport: telemetryTestTransport(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.URL.Scheme, r.URL.Host = collectorURL.Scheme, collectorURL.Host
		return collector.Client().Transport.RoundTrip(r)
	})}
	runner := newStartedRunnerForStopTest(nil, false)
	runner.logger = logmock.New(t)
	runner.telemetry = telemetry.NewTelemetry(client, "test-api-key", "datadoghq.com", observability.ParService)
	runner.executorDone = make(chan struct{})
	draining := make(chan struct{})
	runner.cancelStart = func() { close(draining) }

	// Two concurrent executions finish only after shutdown requests a drain.
	ready := make(chan struct{}, 2)
	finish := make(chan struct{})
	var tasks sync.WaitGroup
	for i := range 2 {
		tasks.Add(1)
		go func() {
			defer tasks.Done()
			ctx := telemetry.WithService(context.Background(), observability.ParService)
			action, ctx := telemetry.StartSpanFromUint64IDs(ctx, observability.ActionRunOperation, uint64(9007199254740993+i), 42)
			child, _ := telemetry.StartSpanFromContext(telemetry.WithService(ctx, observability.RshellService), "rshell.run")
			ready <- struct{}{}
			<-finish
			var actionErr error
			if i == 1 {
				actionErr = errors.New("test action error")
			}
			child.Finish(actionErr)
			action.Finish(actionErr)
		}()
	}
	<-ready
	<-ready
	stopped := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stopped <- runner.StopExecutor(ctx)
	}()
	<-draining
	close(finish)
	tasks.Wait()
	close(runner.executorDone)
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("executor shutdown did not finish")
	}

	// No periodic tick is needed: shutdown must export all finished spans now.
	require.Len(t, payloads, 1)
	traces := <-payloads
	require.Len(t, traces, 2)
	ids := make(map[uint64]bool)
	for _, trace := range traces {
		require.Len(t, trace, 2)
		var action, child span
		for _, s := range trace {
			assert.Positive(t, s.Duration)
			assert.NotZero(t, s.SpanID)
			assert.False(t, ids[s.SpanID])
			ids[s.SpanID] = true
			if s.Name == observability.ActionRunOperation {
				action = s
			} else {
				child = s
			}
		}
		assert.Equal(t, observability.ParService, action.Service)
		assert.Equal(t, observability.RshellService, child.Service)
		assert.Equal(t, action.TraceID, child.TraceID)
		assert.Equal(t, action.SpanID, child.ParentID)
		assert.Equal(t, uint64(42), action.ParentID)
		assert.Contains(t, []uint64{9007199254740993, 9007199254740994}, action.TraceID)
		wantError := int(action.TraceID - 9007199254740993)
		assert.Equal(t, wantError, action.Error)
		assert.Equal(t, wantError, child.Error)
	}
}

type telemetryTestTransport func(*http.Request) (*http.Response, error)

func (f telemetryTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
