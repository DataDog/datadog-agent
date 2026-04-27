// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package errortrackingimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

// makeSender constructs a senderImpl pointed at the given test server URL.
// Tests own the URL/api-key combination so they can assert headers and body
// without going through the Fx config wiring.
func makeSender(t *testing.T, url string) *senderImpl {
	t.Helper()
	return newSenderImpl(
		logmock.New(t),
		&http.Client{Timeout: 5 * time.Second},
		url,
		"test-api-key",
		"7.59.0",
		"test-host",
	)
}

func makeRecord(t *testing.T, msg string, level slog.Level, attrs ...slog.Attr) slog.Record {
	t.Helper()
	r := slog.NewRecord(time.Date(2026, 4, 27, 18, 0, 0, 0, time.UTC), level, msg, 0)
	r.AddAttrs(attrs...)
	return r
}

func TestSend_OneRecord(t *testing.T) {
	var got Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &got))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := makeSender(t, srv.URL)
	rec := makeRecord(t, "boom", slog.LevelError, slog.String("k", "v"))

	require.NoError(t, s.Send(context.Background(), []slog.Record{rec}))

	assert.Equal(t, apiVersion, got.APIVersion)
	assert.Equal(t, requestType, got.RequestType)
	assert.Equal(t, "test-host", got.Host.Hostname)
	assert.Equal(t, "7.59.0", got.Payload.AgentVersion)
	assert.Equal(t, serviceName, got.Payload.Service)
	require.Len(t, got.Payload.Records, 1)
	assert.Equal(t, "boom", got.Payload.Records[0].Message)
	assert.Equal(t, "ERROR", got.Payload.Records[0].Level)
	assert.Equal(t, "v", got.Payload.Records[0].Attrs["k"])
}

func TestSend_Batch(t *testing.T) {
	const batchSize = 30
	var requestCount atomic.Int32
	var got Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &got))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	s := makeSender(t, srv.URL)

	batch := make([]slog.Record, batchSize)
	for i := range batch {
		batch[i] = makeRecord(t, fmt.Sprintf("msg-%d", i), slog.LevelError)
	}

	require.NoError(t, s.Send(context.Background(), batch))

	assert.Equal(t, int32(1), requestCount.Load(), "the sender must POST once per batch — batching belongs to the pipeline, not here")
	assert.Len(t, got.Payload.Records, batchSize)
	for i, rec := range got.Payload.Records {
		assert.Equal(t, fmt.Sprintf("msg-%d", i), rec.Message)
	}
}

func TestSend_5xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := makeSender(t, srv.URL)
	err := s.Send(context.Background(), []slog.Record{makeRecord(t, "boom", slog.LevelError)})

	require.Error(t, err, "5xx must surface as a retryable error so the pipeline retries once")
}

func TestSend_4xxIsTerminal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	s := makeSender(t, srv.URL)
	err := s.Send(context.Background(), []slog.Record{makeRecord(t, "boom", slog.LevelError)})

	assert.NoError(t, err, "4xx is not retryable — Send must report success so the pipeline drops the batch instead of retrying")
}

func TestSend_NetworkError(t *testing.T) {
	// Start a server, capture its URL, then close it so requests fail.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	s := makeSender(t, url)
	err := s.Send(context.Background(), []slog.Record{makeRecord(t, "boom", slog.LevelError)})

	require.Error(t, err)
}

func TestSend_NetworkError_PropagatesContextCancel(t *testing.T) {
	// A handler that blocks until the test releases it; we cancel the context
	// to force the in-flight request to fail with a context error.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	s := makeSender(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())

	var sendErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sendErr = s.Send(ctx, []slog.Record{makeRecord(t, "boom", slog.LevelError)})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()

	require.Error(t, sendErr)
	assert.True(t, errors.Is(sendErr, context.Canceled), "ctx cancellation should propagate as the returned error")
}

func TestSend_PayloadShape(t *testing.T) {
	// Capture the raw body and assert the JSON keys/values match the wire
	// shape Worker 4's ARCH_NOTES_coat_intake.md §2 pinned. Drift here means
	// the receiver-side parser silently drops fields, so this test guards
	// the contract.
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-api-key", r.Header.Get("DD-Api-Key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, apiVersion, r.Header.Get("DD-Telemetry-api-version"))
		assert.Equal(t, requestType, r.Header.Get("DD-Telemetry-request-type"))
		assert.Equal(t, "agent", r.Header.Get("DD-Telemetry-Product"))
		assert.Equal(t, "7.59.0", r.Header.Get("DD-Telemetry-Product-Version"))
		assert.Equal(t, "Datadog Agent/7.59.0", r.Header.Get("User-Agent"))
		raw, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := makeSender(t, srv.URL)
	rec := makeRecord(t, "the message", slog.LevelError,
		slog.String("k1", "v1"),
		slog.Int("count", 7),
	)

	require.NoError(t, s.Send(context.Background(), []slog.Record{rec}))

	var generic map[string]any
	require.NoError(t, json.Unmarshal(raw, &generic))

	assert.Equal(t, apiVersion, generic["api_version"])
	assert.Equal(t, requestType, generic["request_type"])
	assert.Contains(t, generic, "event_time")
	host := generic["host"].(map[string]any)
	assert.Equal(t, "test-host", host["hostname"])

	inner := generic["payload"].(map[string]any)
	assert.Equal(t, "7.59.0", inner["agent_version"])
	assert.Equal(t, "test-host", inner["hostname"])
	assert.Equal(t, serviceName, inner["service"])
	records := inner["records"].([]any)
	require.Len(t, records, 1)
	first := records[0].(map[string]any)
	assert.Equal(t, "the message", first["message"])
	assert.Equal(t, "ERROR", first["level"])
	assert.NotEmpty(t, first["time"])
	attrs := first["attrs"].(map[string]any)
	assert.Equal(t, "v1", attrs["k1"])
	assert.Equal(t, "7", attrs["count"])
}

func TestSend_EmptyBatchSkipsRequest(t *testing.T) {
	called := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := makeSender(t, srv.URL)
	require.NoError(t, s.Send(context.Background(), nil))
	require.NoError(t, s.Send(context.Background(), []slog.Record{}))
	assert.False(t, called.Load(), "an empty batch must not produce an HTTP request")
}

func TestSend_AfterStopIsNoOp(t *testing.T) {
	called := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := makeSender(t, srv.URL)
	s.markStopped()

	require.NoError(t, s.Send(context.Background(), []slog.Record{makeRecord(t, "boom", slog.LevelError)}))
	assert.False(t, called.Load(), "after OnStop, Send must not initiate new requests")
}
