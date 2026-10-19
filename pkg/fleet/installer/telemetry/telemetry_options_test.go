// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTelemetryDefaultEndpoint(t *testing.T) {
	telem := newTelemetry(&http.Client{}, "api", "datadoghq.eu", "test-service")
	require.Len(t, telem.telemetryClient.endpoints, 1)
	assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.eu", telem.telemetryClient.endpoints[0].Host)
}

func TestNewTelemetryWithEndpointURL(t *testing.T) {
	telem := newTelemetry(&http.Client{}, "api", "datadoghq.com", "test-service", WithEndpointURL(" https://relay.example:3840/ "))
	require.Len(t, telem.telemetryClient.endpoints, 1)
	assert.Equal(t, "https://relay.example:3840", telem.telemetryClient.endpoints[0].Host)
	assert.True(t, telem.telemetryClient.siteSupportsTelemetry)
}

func TestNewTelemetryWithEmptyEndpointURLKeepsDefault(t *testing.T) {
	telem := newTelemetry(&http.Client{}, "api", "datadoghq.com", "test-service", WithEndpointURL(""))
	assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com", telem.telemetryClient.endpoints[0].Host)
}

func TestNewTelemetryWithDisabled(t *testing.T) {
	telem := newTelemetry(&http.Client{}, "api", "datadoghq.com", "test-service", WithDisabled())
	assert.False(t, telem.telemetryClient.siteSupportsTelemetry)
}

func TestNewTelemetryGovCloudStaysDisabledWithEndpointURL(t *testing.T) {
	telem := newTelemetry(&http.Client{}, "api", "ddog-gov.com", "test-service", WithEndpointURL("https://relay.example"))
	assert.False(t, telem.telemetryClient.siteSupportsTelemetry)
}

// newUnresponsiveServer returns a server whose handlers never answer until the test ends.
func newUnresponsiveServer(t *testing.T) *httptest.Server {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	return server
}

func finishTestSpan() {
	span, _ := StartSpanFromContext(context.Background(), "options-test")
	span.Finish(nil)
}

func TestStopFlushesToEndpointURL(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == telemetryEndpoint && r.Header.Get("dd-api-key") == "api" {
			received.Add(1)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	telem := newTelemetry(server.Client(), "api", "datadoghq.com", "test-service", WithEndpointURL(server.URL))
	telem.Start()
	finishTestSpan()
	telem.Stop()

	assert.Positive(t, received.Load(), "the final flush should reach the configured endpoint")
}

func TestStopFlushTimeoutBoundsShutdown(t *testing.T) {
	server := newUnresponsiveServer(t)

	const stopFlushTimeout = 100 * time.Millisecond
	telem := newTelemetry(server.Client(), "api", "datadoghq.com", "test-service",
		WithEndpointURL(server.URL), WithStopFlushTimeout(stopFlushTimeout))
	telem.Start()
	finishTestSpan()

	start := time.Now()
	telem.Stop()

	assert.Less(t, time.Since(start), defaultSendPayloadTimeout/2,
		"Stop must not wait for the full send timeout when the intake does not answer")
}

func TestStopBoundsInFlightPeriodicSend(t *testing.T) {
	requestStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case requestStarted <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})

	const stopFlushTimeout = 100 * time.Millisecond
	telem := newTelemetry(server.Client(), "api", "datadoghq.com", "test-service",
		WithEndpointURL(server.URL), WithStopFlushTimeout(stopFlushTimeout))
	telem.flushInterval = 10 * time.Millisecond
	telem.Start()
	finishTestSpan()

	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("the periodic flush never reached the intake")
	}

	start := time.Now()
	telem.Stop()

	assert.Less(t, time.Since(start), defaultSendPayloadTimeout/2,
		"Stop must cancel a periodic send that is already waiting on the intake")
}

func TestSendPayloadStopsWhenContextCanceled(t *testing.T) {
	server := newUnresponsiveServer(t)
	c := newClient(server.Client(), []*endpoint{{Host: server.URL, APIKey: "api"}}, "test-service", false, true)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	c.sendPayloadWithContext(ctx, requestTypeLogs, LogPayload{})

	assert.Less(t, time.Since(start), defaultSendPayloadTimeout/2, "canceling the context must end the send")
}
