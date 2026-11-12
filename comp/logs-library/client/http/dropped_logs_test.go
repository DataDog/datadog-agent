// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func logLine(source, service string, size int) *message.MessageMetadata {
	origin := message.NewOrigin(sources.NewLogSource(source, &config.LogsConfig{Source: source, Service: service}))
	return &message.MessageMetadata{Origin: origin, RawDataLen: size}
}

func TestStatusErrorKeepsMessageAndSentinel(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusForbidden, Status: "403 Forbidden"}

	clientErr := newStatusError(errClient, resp)
	assert.Equal(t, fmt.Errorf("%w: %s", errClient, resp.Status).Error(), clientErr.Error(),
		"the log line and any caller matching on the message must see the old text")
	assert.ErrorIs(t, clientErr, errClient)

	retryable := client.NewRetryableError(newStatusError(errServer, resp))
	assert.ErrorIs(t, retryable, errServer)
	assert.Equal(t, metrics.DropReasonAuthRejected, dropReasonFor(retryable),
		"the status must survive the RetryableError wrapper")
}

func TestDropReasonFor(t *testing.T) {
	status := func(code int) error {
		return newStatusError(errClient, &http.Response{StatusCode: code, Status: http.StatusText(code)})
	}
	tests := []struct {
		err  error
		want metrics.DropReason
	}{
		{status(http.StatusUnauthorized), metrics.DropReasonAuthRejected},
		{status(http.StatusForbidden), metrics.DropReasonAuthRejected},
		{status(http.StatusRequestEntityTooLarge), metrics.DropReasonPayloadTooLarge},
		{status(http.StatusBadRequest), metrics.DropReasonBadRequest},
		{client.NewRetryableError(newStatusError(errServer, &http.Response{StatusCode: 503})), metrics.DropReasonSendFailed},
		{fmt.Errorf("wrapped: %w", errCredentialNotReady), metrics.DropReasonCredentialUnavailable},
		{client.NewRetryableError(errors.New("connection refused")), metrics.DropReasonSendFailed},
	}
	for _, tc := range tests {
		t.Run(tc.err.Error(), func(t *testing.T) {
			assert.Equal(t, tc.want, dropReasonFor(tc.err))
		})
	}
}

func TestDropTargetFor(t *testing.T) {
	endpoint := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "intake.example.com"})
	mrf := config.NewMockEndpointWithOptions(map[string]interface{}{"host": "mrf.example.com"})
	mrf.IsMRF = true

	tests := []struct {
		name        string
		endpoint    config.Endpoint
		meta        *client.DestinationMetadata
		shouldRetry bool
		want        metrics.DropTargetKind
	}{
		{"main endpoint", endpoint, client.NewDestinationMetadata("logs", "1", "reliable", "0", ""), true, metrics.DropTargetPrimary},
		{"reliable additional", endpoint, client.NewDestinationMetadata("logs", "1", "reliable", "1", ""), true, metrics.DropTargetAdditional},
		{"is_reliable false", endpoint, client.NewDestinationMetadata("logs", "1", "unreliable", "0", ""), false, metrics.DropTargetBestEffort},
		{"no metadata, retrying", endpoint, client.NewNoopDestinationMetadata(), true, metrics.DropTargetPrimary},
		{"no metadata, best effort", endpoint, client.NewNoopDestinationMetadata(), false, metrics.DropTargetBestEffort},
		{"mrf wins", mrf, client.NewDestinationMetadata("logs", "1", "reliable", "1", ""), true, metrics.DropTargetMRF},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dropTargetFor(tc.endpoint, tc.meta, tc.shouldRetry)
			assert.Equal(t, tc.want, got.Kind)
			assert.Equal(t, tc.endpoint.Host, got.Host)
		})
	}
}

// A rejected payload is attributed per source and service, under the reason the
// intake gave, while event-platform messages riding the same code path are not.
func TestPermanentErrorRecordsDroppedLogs(t *testing.T) {
	tests := []struct {
		code int
		want metrics.DropReason
	}{
		{http.StatusForbidden, metrics.DropReasonAuthRejected},
		{http.StatusRequestEntityTooLarge, metrics.DropReasonPayloadTooLarge},
		{http.StatusBadRequest, metrics.DropReasonBadRequest},
	}
	for _, tc := range tests {
		t.Run(http.StatusText(tc.code), func(t *testing.T) {
			metrics.ResetDroppedLogsForTest()
			t.Cleanup(metrics.ResetDroppedLogsForTest)

			server := NewTestServer(tc.code, configmock.New(t))
			defer server.Stop()
			input, output := make(chan *message.Payload), make(chan *message.Payload)
			server.Destination.Start(input, output, nil)

			input <- &message.Payload{
				MessageMetas: []*message.MessageMetadata{
					logLine("nginx", "web", 100),
					logLine("nginx", "web", 50),
					logLine("redis", "cache", 10),
					{RawDataLen: 999}, // an event-platform message: no Origin
				},
				Encoded: []byte("payload"),
			}
			<-output

			got := metrics.DroppedLogsSnapshot()
			require.Len(t, got, 2)
			assert.Equal(t, "nginx", got[0].Source)
			assert.Equal(t, int64(2), got[0].Logs)
			assert.Equal(t, int64(150), got[0].Bytes)
			assert.Equal(t, tc.want, got[0].Reason)
			assert.Equal(t, metrics.DropTargetPrimary, got[0].Kind)
			assert.Equal(t, server.Destination.host, got[0].Destination)
			assert.Equal(t, "redis", got[1].Source)
		})
	}
}

// Retryable errors on a reliable destination block and retry; nothing is lost yet.
func TestRetriedPayloadIsNotRecorded(t *testing.T) {
	metrics.ResetDroppedLogsForTest()
	t.Cleanup(metrics.ResetDroppedLogsForTest)

	respondChan := make(chan int)
	server := NewTestServerWithOptions(http.StatusServiceUnavailable, 1, true, respondChan, configmock.New(t))
	input, output := make(chan *message.Payload), make(chan *message.Payload)
	isRetrying := make(chan bool, 1)
	server.Destination.Start(input, output, isRetrying)

	input <- &message.Payload{MessageMetas: []*message.MessageMetadata{logLine("nginx", "web", 10)}, Encoded: []byte("p")}
	// Two responses guarantee the destination is in its retry loop.
	<-respondChan
	<-respondChan
	require.True(t, <-isRetrying)

	assert.Empty(t, metrics.DroppedLogsSnapshot())

	server.ChangeStatus(http.StatusOK)
	// Drain the retries still in flight.
	for {
		if (<-respondChan) == http.StatusOK {
			break
		}
	}
	<-output
	assert.False(t, <-isRetrying)
	assert.Empty(t, metrics.DroppedLogsSnapshot(), "a payload delivered after retries was never lost")
	server.Stop()
}
