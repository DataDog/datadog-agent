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

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

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
	err := transaction.Process(context.Background(), mockConfig, log, secrets, client)
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
	err := transaction.Process(context.Background(), mockConfig, log, secrets, client)
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
	err := transaction.Process(context.Background(), mockConfig, log, secrets, client)
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
	err := transaction.Process(context.Background(), mockConfig, log, secrets, client)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "error \"503 Service Unavailable\" while sending transaction")

	errorCode = http.StatusBadRequest
	err = transaction.Process(context.Background(), mockConfig, log, secrets, client)
	assert.NoError(t, err)

	errorCode = http.StatusRequestEntityTooLarge
	err = transaction.Process(context.Background(), mockConfig, log, secrets, client)
	assert.NoError(t, err)
	assert.Equal(t, transaction.ErrorCount, 1)

	errorCode = http.StatusForbidden
	err = transaction.Process(context.Background(), mockConfig, log, secrets, client)
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
	err := transaction.Process(ctx, mockConfig, log, secrets, client)
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
	err := transaction.Process(context.Background(), mockConfig, log, secrets, client)
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

	err := transaction.Process(context.Background(), mockConfig, log, secrets, client)
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

	err := transaction.Process(context.Background(), mockConfig, log, secrets, client)

	assert.NoError(t, err, "a 403 with no secrets backend should drop the transaction, not reschedule it")
	assert.Equal(t, 0, transaction.ErrorCount, "ErrorCount should not be incremented when the transaction is dropped")
	assert.Equal(t, droppedBefore+1, TransactionsDropped.Value(), "TransactionsDropped should be incremented")
	assert.Equal(t, droppedByEndpointBefore+1, TransactionsDroppedByEndpoint.Get("test").(*expvar.Int).Value(), "TransactionsDroppedByEndpoint should be incremented for the endpoint")
}
