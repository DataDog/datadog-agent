// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import (
	"context"
	"expvar"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// stubPendingDelegatedAuthResolver is a minimal Authorizer + PendingDelegatedAuthChecker used to
// simulate a domain resolver that's WIF-managed, without pulling in the full resolver package.
type stubPendingDelegatedAuthResolver struct {
	pending bool
}

func (s *stubPendingDelegatedAuthResolver) Authorize(uint, http.Header, log.Component) {}
func (s *stubPendingDelegatedAuthResolver) HasPendingDelegatedAuth() bool              { return s.pending }

func TestNewHTTPTransaction(t *testing.T) {
	before := time.Now()
	transaction := NewHTTPTransaction()
	after := time.Now()

	assert.NotNil(t, transaction)

	assert.True(t, transaction.CreatedAt.After(before) || transaction.CreatedAt.Equal(before))
	assert.True(t, transaction.CreatedAt.Before(after) || transaction.CreatedAt.Equal(after))
}

func TestGetCreatedAt(t *testing.T) {
	transaction := NewHTTPTransaction()

	assert.NotNil(t, transaction)
	assert.Equal(t, transaction.CreatedAt, transaction.GetCreatedAt())
}

func TestProcess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint.Route = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = NewBytesPayloadWithoutMetaData(payload)

	client := &http.Client{}

	mockConfig := configmock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	err := transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)
	assert.NoError(t, err)
}

func TestProcessInvalidDomain(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "://invalid"
	transaction.Endpoint.Route = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = NewBytesPayloadWithoutMetaData(payload)

	client := &http.Client{}

	mockConfig := configmock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	err := transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)
	assert.NoError(t, err)
}

func TestProcessNetworkError(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "http://localhost:1234"
	transaction.Endpoint.Route = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = NewBytesPayloadWithoutMetaData(payload)

	client := &http.Client{}

	mockConfig := configmock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	err := transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)
	assert.NotNil(t, err)
}

func TestProcessHTTPError(t *testing.T) {
	errorCode := http.StatusServiceUnavailable

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(errorCode)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint.Route = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = NewBytesPayloadWithoutMetaData(payload)

	client := &http.Client{}

	mockConfig := configmock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	secrets.SetRefreshHook(func() bool {
		return true
	})
	err := transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "error \"503 Service Unavailable\" while sending transaction")

	errorCode = http.StatusBadRequest
	err = transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)
	assert.NoError(t, err)

	errorCode = http.StatusRequestEntityTooLarge
	err = transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)
	assert.NoError(t, err)
	assert.Equal(t, transaction.ErrorCount, 1)

	errorCode = http.StatusForbidden
	err = transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "API Key invalid")

	assert.Equal(t, transaction.ErrorCount, 2)
}

func TestProcessCancel(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "example.com"
	transaction.Endpoint.Route = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = NewBytesPayloadWithoutMetaData(payload)

	client := &http.Client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mockConfig := configmock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	err := transaction.Process(ctx, mockConfig, log, secrets, nil, client, nil)
	assert.NoError(t, err)
}

func Test_truncateBodyForLog(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want []byte
	}{
		{
			name: "body is datadog",
			body: []byte("datadog"),
			want: []byte("datadog"),
		},
		{
			name: "body is 1000 bytes",
			body: []byte(strings.Repeat("a", 1000)),
			want: append([]byte(strings.Repeat("a", 1000)), []byte("...")...),
		},
		{
			name: "body is 1001 bytes",
			body: []byte(strings.Repeat("a", 1001)),
			want: append([]byte(strings.Repeat("a", 1000)), []byte("...")...),
		},
		{
			name: "body is empty",
			body: []byte{},
			want: []byte{},
		},
		{
			name: "body is nil",
			body: nil,
			want: []byte{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateBodyForLog(tt.body); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("truncateBodyForLog() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockAuthorizer struct{}

func (mockAuthorizer) Authorize(_ uint, h http.Header, _ log.Component) {
	h.Set("DD-Api-Key", "secret")
}

// TestProcessDoesNotMutateHeaders verifies that internalProcess does not add the
// API key (or any other header) to the transaction's own Headers field.
func TestProcessDoesNotMutateHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "secret", r.Header.Get("DD-Api-Key"), "request should carry the API key")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint.Route = "/"
	transaction.Payload = NewBytesPayloadWithoutMetaData([]byte("payload"))
	transaction.Resolver = mockAuthorizer{}
	transaction.Headers.Set("Content-Type", "application/json")

	headersBefore := transaction.Headers.Clone()

	client := &http.Client{}
	mockConfig := configmock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	err := transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)
	assert.NoError(t, err)

	assert.Equal(t, headersBefore, transaction.Headers, "t.Headers must not be mutated by Process")
	assert.Empty(t, transaction.Headers.Get("DD-Api-Key"), "API key must not appear in t.Headers")
}

func TestTransaction403TriggersSecretRefresh(t *testing.T) {
	triggered := false

	secrets := secretsmock.New(t)
	secrets.SetRefreshHook(func() bool {
		triggered = true
		return true
	})

	// test server that returns 403 for all reequests
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint.Route = "/endpoint/test"
	transaction.Payload = NewBytesPayloadWithoutMetaData([]byte("test payload"))

	client := &http.Client{}
	mockConfig := configmock.New(t)
	log := logmock.New(t)

	err := transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)
	assert.NotNil(t, err)

	assert.True(t, triggered, "secrets.Refresh(false) should be called when transaction receives 403")
}

func TestTransaction403DropsWhenNoSecrets(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint.Route = "/endpoint/test"
	transaction.Endpoint.Name = "test"
	transaction.Payload = NewBytesPayloadWithoutMetaData([]byte("test payload"))

	secrets := secretsmock.New(t)
	secrets.SetRefreshHook(func() bool {
		// The secrets have not been set up.
		return false
	})

	client := &http.Client{}
	mockConfig := configmock.New(t)
	log := logmock.New(t)

	droppedBefore := TransactionsDropped.Value()
	droppedByEndpointBefore := int64(0)
	if v := TransactionsDroppedByEndpoint.Get("test"); v != nil {
		droppedByEndpointBefore = v.(*expvar.Int).Value()
	}

	err := transaction.Process(context.Background(), mockConfig, log, secrets, nil, client, nil)

	assert.NoError(t, err, "a 403 with no secrets backend should drop the transaction, not reschedule it")
	assert.Equal(t, 0, transaction.ErrorCount, "ErrorCount should not be incremented when the transaction is dropped")
	assert.Equal(t, droppedBefore+1, TransactionsDropped.Value(), "TransactionsDropped should be incremented")
	assert.Equal(t, droppedByEndpointBefore+1, TransactionsDroppedByEndpoint.Get("test").(*expvar.Int).Value(), "TransactionsDroppedByEndpoint should be incremented for the endpoint")
}

// pointCountTelemetryRecorder is a minimal PointCountTelemetry that records
// every call so tests can assert on point.sent / point.dropped accounting.
type pointCountTelemetryRecorder struct {
	sent    int
	dropped int
}

func (r *pointCountTelemetryRecorder) OnPointSuccessfullySent(count int) { r.sent += count }
func (r *pointCountTelemetryRecorder) OnPointDropped(count int)          { r.dropped += count }

func newTransactionForStatusTest(domain string, pointCount int) *HTTPTransaction {
	tr := NewHTTPTransaction()
	tr.Domain = domain
	tr.Endpoint.Route = "/endpoint/test"
	tr.Endpoint.Name = "test"
	tr.Payload = NewBytesPayload([]byte("test payload"), pointCount)
	return tr
}

func TestProcessPointCountTelemetry(t *testing.T) {
	cases := []struct {
		name                           string
		status                         int
		pointCount                     int
		setRefreshHook                 bool
		refreshSucceeds                bool
		pendingDelegatedAuth           bool
		wantErr                        bool
		wantSent                       int
		wantDropped                    int
		wantDelegatedAuthRefreshCalled bool
	}{
		{
			name:       "2xx credits point.sent",
			status:     http.StatusOK,
			pointCount: 17,
			wantSent:   17,
		},
		{
			name:        "400 drop credits point.dropped",
			status:      http.StatusBadRequest,
			pointCount:  9,
			wantDropped: 9,
		},
		{
			name:        "413 drop credits point.dropped",
			status:      http.StatusRequestEntityTooLarge,
			pointCount:  4,
			wantDropped: 4,
		},
		{
			name:            "403 with failed key refresh credits point.dropped",
			status:          http.StatusForbidden,
			pointCount:      3,
			setRefreshHook:  true,
			refreshSucceeds: false,
			wantDropped:     3,
		},
		{
			name:            "403 with successful key refresh is retryable",
			status:          http.StatusForbidden,
			pointCount:      12,
			setRefreshHook:  true,
			refreshSucceeds: true,
			wantErr:         true,
		},
		{
			name:       "5xx is retryable",
			status:     http.StatusServiceUnavailable,
			pointCount: 6,
			wantErr:    true,
		},
		{
			// A WIF-managed domain that hasn't resolved its first key yet (or is between
			// retries) must retry a 403 rather than drop it, and nudge delegated auth to try
			// sooner - see PendingDelegatedAuthChecker in transaction.go.
			name:                           "403 with delegated auth pending is retryable",
			status:                         http.StatusForbidden,
			pointCount:                     5,
			pendingDelegatedAuth:           true,
			wantErr:                        true,
			wantDelegatedAuthRefreshCalled: true,
		},
		{
			// A plain 403 with no secrets refresh hook and no delegated-auth-managed domain
			// still drops permanently, exactly as before this feature existed.
			name:        "403 without delegated auth pending still drops",
			status:      http.StatusForbidden,
			pointCount:  2,
			wantDropped: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer ts.Close()

			tr := newTransactionForStatusTest(ts.URL, tc.pointCount)
			if tc.pendingDelegatedAuth {
				tr.Resolver = &stubPendingDelegatedAuthResolver{pending: true}
			}
			rec := &pointCountTelemetryRecorder{}

			secrets := secretsmock.New(t)
			if tc.setRefreshHook {
				secrets.SetRefreshHook(func() bool { return tc.refreshSucceeds })
			}

			var delegatedAuthRefreshCalled bool
			delegatedAuth := &delegatedauthmock.Mock{
				RefreshFunc: func() bool {
					delegatedAuthRefreshCalled = true
					return true
				},
			}

			err := tr.Process(context.Background(), configmock.New(t), logmock.New(t),
				secrets, delegatedAuth, &http.Client{}, rec)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.wantSent, rec.sent, "point.sent")
			assert.Equal(t, tc.wantDropped, rec.dropped, "point.dropped")
			assert.Equal(t, tc.wantDelegatedAuthRefreshCalled, delegatedAuthRefreshCalled, "delegated auth Refresh() called")
		})
	}
}
