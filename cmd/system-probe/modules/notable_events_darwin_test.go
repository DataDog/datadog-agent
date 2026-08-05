// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package modules

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/notableevents"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

type fakeNotableEventsCollector struct {
	events       []notableevents.Event
	acked        []string
	startErr     error
	closeErr     error
	ackErr       error
	startCalls   int
	closeCalls   int
	pendingCalls int
}

// Start records lifecycle activation and returns the configured failure.
func (f *fakeNotableEventsCollector) Start() error {
	f.startCalls++
	return f.startErr
}

// Close records lifecycle cleanup and returns the configured failure.
func (f *fakeNotableEventsCollector) Close() error {
	f.closeCalls++
	return f.closeErr
}

// Pending records retrieval and returns the fake event snapshot.
func (f *fakeNotableEventsCollector) Pending() []notableevents.Event {
	f.pendingCalls++
	return f.events
}

// Ack records delivered IDs and returns the configured persistence failure.
func (f *fakeNotableEventsCollector) Ack(ids []string) error {
	f.acked = append([]string(nil), ids...)
	return f.ackErr
}

// notableEventsTestHandler constructs the module routes around a supplied fake collector.
func notableEventsTestHandler(t *testing.T, collector notableEventsCollector) http.Handler {
	t.Helper()
	parent := http.NewServeMux()
	router := module.NewRouter("notable_events", parent)
	require.NoError(t, (&notableEventsModule{collector: collector}).Register(router))
	return parent
}

// TestNotableEventsModuleFactoryLifecycle verifies construction starts and shutdown closes the collector.
func TestNotableEventsModuleFactoryLifecycle(t *testing.T) {
	collector := &fakeNotableEventsCollector{}
	previousFactory := newNotableEventsCollector
	newNotableEventsCollector = func() (notableEventsCollector, error) {
		return collector, nil
	}
	t.Cleanup(func() { newNotableEventsCollector = previousFactory })

	created, err := createNotableEventsModule(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	require.Equal(t, 1, collector.startCalls)

	created.Close()
	require.Equal(t, 1, collector.closeCalls)
}

// TestNotableEventsModuleFactoryClosesAfterStartFailure verifies partial startup releases collector resources.
func TestNotableEventsModuleFactoryClosesAfterStartFailure(t *testing.T) {
	collector := &fakeNotableEventsCollector{startErr: errors.New("start failed")}
	previousFactory := newNotableEventsCollector
	newNotableEventsCollector = func() (notableEventsCollector, error) {
		return collector, nil
	}
	t.Cleanup(func() { newNotableEventsCollector = previousFactory })

	created, err := createNotableEventsModule(nil, module.FactoryDependencies{})
	require.ErrorContains(t, err, "start notable events collector")
	require.Nil(t, created)
	require.Equal(t, 1, collector.startCalls)
	require.Equal(t, 1, collector.closeCalls)
}

// TestNotableEventsCheckReturnsPendingWithoutConsuming verifies checks return repeatable pending snapshots.
func TestNotableEventsCheckReturnsPendingWithoutConsuming(t *testing.T) {
	event := notableevents.Event{
		ID:        "event-1",
		Timestamp: time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC),
		EventType: "Application crash",
		Title:     "Application crash: example",
		Message:   "An application crashed unexpectedly",
		Custom:    map[string]interface{}{"scope": "system"},
	}
	collector := &fakeNotableEventsCollector{events: []notableevents.Event{event}}
	handler := notableEventsTestHandler(t, collector)

	for range 2 {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/notable_events/check", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
		assert.JSONEq(t, `[{
			"id":"event-1",
			"timestamp":"2026-07-21T12:00:00Z",
			"event_type":"Application crash",
			"title":"Application crash: example",
			"message":"An application crashed unexpectedly",
			"custom":{"scope":"system"}
		}]`, recorder.Body.String())
	}
	assert.Equal(t, 2, collector.pendingCalls)
	assert.Empty(t, collector.acked)
}

// TestNotableEventsAck verifies valid delivery acknowledgements reach the collector.
func TestNotableEventsAck(t *testing.T) {
	collector := &fakeNotableEventsCollector{}
	handler := notableEventsTestHandler(t, collector)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/notable_events/ack", strings.NewReader(`{"ids":["event-1","event-2"]}`))

	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{}`, recorder.Body.String())
	assert.Equal(t, []string{"event-1", "event-2"}, collector.acked)
}

// TestNotableEventsAckValidation verifies malformed and unsafe acknowledgement bodies are rejected.
func TestNotableEventsAckValidation(t *testing.T) {
	tests := []struct {
		name   string
		body   []byte
		status int
	}{
		{name: "malformed JSON", body: []byte(`{"ids":`), status: http.StatusBadRequest},
		{name: "unknown field", body: []byte(`{"ids":[],"extra":true}`), status: http.StatusBadRequest},
		{name: "missing ids", body: []byte(`{}`), status: http.StatusBadRequest},
		{name: "null ids", body: []byte(`{"ids":null}`), status: http.StatusBadRequest},
		{name: "empty id", body: []byte(`{"ids":[""]}`), status: http.StatusBadRequest},
		{name: "whitespace id", body: []byte(`{"ids":[" "]}`), status: http.StatusBadRequest},
		{
			name:   "too many ids",
			body:   []byte(`{"ids":[` + strings.Repeat(`"id",`, maxNotableEventsAckIDs) + `"id"]}`),
			status: http.StatusBadRequest,
		},
		{
			name:   "id too long",
			body:   []byte(`{"ids":["` + strings.Repeat("x", maxNotableEventIDLength+1) + `"]}`),
			status: http.StatusBadRequest,
		},
		{name: "multiple values", body: []byte(`{"ids":[]} {}`), status: http.StatusBadRequest},
		{
			name:   "body too large",
			body:   append([]byte(`{"ids":["`), append(bytes.Repeat([]byte("x"), maxNotableEventsAckBodyBytes), []byte(`"]}`)...)...),
			status: http.StatusRequestEntityTooLarge,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collector := &fakeNotableEventsCollector{}
			handler := notableEventsTestHandler(t, collector)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/notable_events/ack", bytes.NewReader(test.body)))

			assert.Equal(t, test.status, recorder.Code)
			assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
			assert.Contains(t, recorder.Body.String(), `"error"`)
			assert.Empty(t, collector.acked)
		})
	}
}

// TestNotableEventsAckFailure verifies persistence failures return a safe server error.
func TestNotableEventsAckFailure(t *testing.T) {
	collector := &fakeNotableEventsCollector{ackErr: errors.New("persistence failed")}
	handler := notableEventsTestHandler(t, collector)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/notable_events/ack", strings.NewReader(`{"ids":["event-1"]}`)))

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.JSONEq(t, `{"error":"failed to acknowledge notable events"}`, recorder.Body.String())
}

// TestNotableEventsRoutesAreMethodSpecific verifies endpoints reject unsupported HTTP methods.
func TestNotableEventsRoutesAreMethodSpecific(t *testing.T) {
	handler := notableEventsTestHandler(t, &fakeNotableEventsCollector{})
	for _, test := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/notable_events/check"},
		{method: http.MethodGet, path: "/notable_events/ack"},
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
		assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
	}
}
