// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
package forwarder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewHTTPTransaction(t *testing.T) {
	before := time.Now()
	transaction := NewHTTPTransaction()
	after := time.Now()

	assert.NotNil(t, transaction)

	assert.True(t, transaction.createdAt.After(before) || transaction.createdAt.Equal(before))
	assert.True(t, transaction.createdAt.Before(after) || transaction.createdAt.Equal(after))
}

func TestGetCreatedAt(t *testing.T) {
	transaction := NewHTTPTransaction()

	assert.NotNil(t, transaction)
	assert.Equal(t, transaction.createdAt, transaction.GetCreatedAt())
}

func TestProcess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	transaction := NewHTTPTransaction()
	transaction.Domain = ts.URL
	transaction.Endpoint = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = &payload

	client := &http.Client{}

	err := transaction.Process(context.Background(), client)
	assert.Nil(t, err)
}

func TestProcessInvalidDomain(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "://invalid"
	transaction.Endpoint = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = &payload

	client := &http.Client{}

	err := transaction.Process(context.Background(), client)
	assert.Nil(t, err)
}

func TestProcessNetworkError(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "http://localhost:1234"
	transaction.Endpoint = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = &payload

	client := &http.Client{}

	err := transaction.Process(context.Background(), client)
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
	transaction.Endpoint = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = &payload

	client := &http.Client{}

	err := transaction.Process(context.Background(), client)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "error \"503 Service Unavailable\" while sending transaction")

	errorCode = http.StatusBadRequest
	err = transaction.Process(context.Background(), client)
	assert.Nil(t, err)

	errorCode = http.StatusRequestEntityTooLarge
	err = transaction.Process(context.Background(), client)
	assert.Nil(t, err)
	assert.Equal(t, transaction.ErrorCount, 1)

	errorCode = http.StatusForbidden
	err = transaction.Process(context.Background(), client)
	assert.Nil(t, err)
	assert.Equal(t, transaction.ErrorCount, 1)
}

func TestProcessCancel(t *testing.T) {
	transaction := NewHTTPTransaction()
	transaction.Domain = "example.com"
	transaction.Endpoint = "/endpoint/test"
	payload := []byte("test payload")
	transaction.Payload = &payload

	client := &http.Client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := transaction.Process(ctx, client)
	assert.Nil(t, err)
}
