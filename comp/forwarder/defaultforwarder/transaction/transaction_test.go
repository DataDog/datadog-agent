// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transaction

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
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
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint.Route = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = NewBytesPayloadWithoutMetaData(payload)

	client := &http.Client{}

	mockConfig := pkgconfig.Mock(t)
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	err := transaction.Process(context.Background(), mockConfig, log, client)
	assert.Nil(t, err)
}

func TestProcessInvalidDomain(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "://invalid"
	transaction.Endpoint.Route = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = NewBytesPayloadWithoutMetaData(payload)

	client := &http.Client{}

	mockConfig := pkgconfig.Mock(t)
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	err := transaction.Process(context.Background(), mockConfig, log, client)
	assert.Nil(t, err)
}

func TestProcessNetworkError(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "http://localhost:1234"
	transaction.Endpoint.Route = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = NewBytesPayloadWithoutMetaData(payload)

	client := &http.Client{}

	mockConfig := pkgconfig.Mock(t)
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	err := transaction.Process(context.Background(), mockConfig, log, client)
	assert.NotNil(t, err)
}

func TestProcessHTTPError(t *testing.T) {
	errorCode := http.StatusServiceUnavailable

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(errorCode)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint.Route = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = NewBytesPayloadWithoutMetaData(payload)

	client := &http.Client{}

	mockConfig := pkgconfig.Mock(t)
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	err := transaction.Process(context.Background(), mockConfig, log, client)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "error \"503 Service Unavailable\" while sending transaction")

	errorCode = http.StatusBadRequest
	err = transaction.Process(context.Background(), mockConfig, log, client)
	assert.Nil(t, err)

	errorCode = http.StatusRequestEntityTooLarge
	err = transaction.Process(context.Background(), mockConfig, log, client)
	assert.Nil(t, err)
	assert.Equal(t, transaction.ErrorCount, 1)

	errorCode = http.StatusForbidden
	err = transaction.Process(context.Background(), mockConfig, log, client)
	assert.Nil(t, err)
	assert.Equal(t, transaction.ErrorCount, 1)
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

	mockConfig := pkgconfig.Mock(t)
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	err := transaction.Process(ctx, mockConfig, log, client)
	assert.Nil(t, err)
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
