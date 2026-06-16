// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// TestSyncDestinationRetryRecovers verifies that a retryable failure (5xx) is
// retried once and delivered when the retry succeeds. This is the behavior that
// lets an idle, CPU-throttled instance's send reach Datadog once CPU is restored.
func TestSyncDestinationRetryRecovers(t *testing.T) {
	cfg := configmock.New(t)
	respondChan := make(chan int)
	server := NewTestServerWithOptions(http.StatusInternalServerError, 1, false, respondChan, cfg)
	dest := NewSyncDestination(server.Endpoint, JSONContentType, server.DestCtx, nil, client.NewNoopDestinationMetadata(), cfg)

	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	dest.Start(input, output, nil)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}

	// First attempt fails with a retryable 500.
	assert.Equal(t, http.StatusInternalServerError, <-respondChan)
	// Restore the endpoint before the single retry fires (CPU is back).
	server.ChangeStatus(http.StatusOK)
	// The retry succeeds and the payload is released exactly once.
	assert.Equal(t, http.StatusOK, <-respondChan)
	<-output

	// No third attempt is made.
	select {
	case <-respondChan:
		t.Fatal("payload should be retried at most once")
	case <-time.After(500 * time.Millisecond):
	}

	server.Stop()
}

// TestSyncDestinationDropsAfterOneRetry verifies that a persistently retryable
// failure is retried exactly once and then dropped, so one failing payload
// cannot starve the payloads queued behind it on this serial destination.
func TestSyncDestinationDropsAfterOneRetry(t *testing.T) {
	cfg := configmock.New(t)
	respondChan := make(chan int)
	server := NewTestServerWithOptions(http.StatusInternalServerError, 1, false, respondChan, cfg)
	dest := NewSyncDestination(server.Endpoint, JSONContentType, server.DestCtx, nil, client.NewNoopDestinationMetadata(), cfg)

	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	dest.Start(input, output, nil)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}

	// Two attempts: the initial send and a single retry, both retryable 500s.
	assert.Equal(t, http.StatusInternalServerError, <-respondChan)
	assert.Equal(t, http.StatusInternalServerError, <-respondChan)

	// The payload is then dropped and released exactly once.
	<-output

	// No third attempt is made.
	select {
	case <-respondChan:
		t.Fatal("payload should be retried at most once")
	case <-time.After(500 * time.Millisecond):
	}

	server.Stop()
}

// TestSyncDestinationDropsNonRetryableErrors verifies that a non-retryable
// failure (4xx) is still dropped after a single attempt rather than retried.
func TestSyncDestinationDropsNonRetryableErrors(t *testing.T) {
	cfg := configmock.New(t)
	respondChan := make(chan int)
	server := NewTestServerWithOptions(http.StatusBadRequest, 1, false, respondChan, cfg)
	dest := NewSyncDestination(server.Endpoint, JSONContentType, server.DestCtx, nil, client.NewNoopDestinationMetadata(), cfg)

	input := make(chan *message.Payload)
	output := make(chan *message.Payload)
	dest.Start(input, output, nil)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{}, Encoded: []byte("yo")}

	// Exactly one attempt, then the payload is released (dropped).
	assert.Equal(t, http.StatusBadRequest, <-respondChan)
	<-output

	// No further attempts should be made.
	select {
	case <-respondChan:
		t.Fatal("non-retryable error should not be retried")
	case <-time.After(500 * time.Millisecond):
	}

	server.Stop()
}
