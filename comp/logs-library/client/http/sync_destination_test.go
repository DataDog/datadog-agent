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

// TestSyncDestinationRetriesRetryableErrors verifies that a retryable failure
// (5xx) is retried until it succeeds instead of being dropped after one attempt.
// This is the behavior that lets a coldstart instance's buffered logs reach
// Datadog once CPU is restored. See SVLS-9268.
func TestSyncDestinationRetriesRetryableErrors(t *testing.T) {
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
	// A second attempt proves the payload was retried, not dropped.
	assert.Equal(t, http.StatusInternalServerError, <-respondChan)

	// The payload must not have been released yet — the send hasn't succeeded.
	select {
	case <-output:
		t.Fatal("payload was released before the send succeeded")
	default:
	}

	// Recover: the next attempt succeeds and the payload is released exactly once.
	server.ChangeStatus(http.StatusOK)
	for (<-respondChan) != http.StatusOK { //nolint:revive
	}
	<-output

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
