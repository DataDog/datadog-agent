// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBlockingDestinationSend(t *testing.T) {
	tests := []struct {
		status    int
		wantErr   bool
		retryable bool
	}{
		{status: http.StatusOK},
		{status: http.StatusBadRequest, wantErr: true},
		{status: http.StatusUnauthorized, wantErr: true},
		{status: http.StatusForbidden, wantErr: true},
		{status: http.StatusRequestEntityTooLarge, wantErr: true},
		{status: http.StatusNotFound, wantErr: true, retryable: true},
		{status: http.StatusTooManyRequests, wantErr: true, retryable: true},
		{status: http.StatusInternalServerError, wantErr: true, retryable: true},
		{status: http.StatusServiceUnavailable, wantErr: true, retryable: true},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			cfg := configmock.New(t)
			responses := make(chan int, 10)
			server := NewTestServerWithOptions(tt.status, 1, true, responses, cfg)
			defer server.Stop()
			d := NewBlockingDestination(server.Endpoint, JSONContentType, client.NewNoopDestinationMetadata(), cfg)

			err := d.Send(context.Background(), &message.Payload{Encoded: []byte("payload")})

			assert.Len(t, responses, 1, "Send must send the payload exactly once")
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			var retryable *client.RetryableError
			assert.Equal(t, tt.retryable, errors.As(err, &retryable))
			assert.Contains(t, err.Error(), strconv.Itoa(tt.status))
		})
	}
}

func TestBlockingDestinationSendUsesCallerContext(t *testing.T) {
	cfg := configmock.New(t)
	responses := make(chan int, 10)
	server := NewTestServerWithOptions(http.StatusOK, 1, true, responses, cfg)
	defer server.Stop()
	d := NewBlockingDestination(server.Endpoint, JSONContentType, client.NewNoopDestinationMetadata(), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := d.Send(ctx, &message.Payload{Encoded: []byte("payload")})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, responses, "a cancelled request must not reach the intake")
}

func TestBlockingDestinationSendTagsTheSourceOfItsMetadata(t *testing.T) {
	bytesSent, encodedBytesSent := metrics.TlmBytesSent, metrics.TlmEncodedBytesSent
	t.Cleanup(func() { metrics.TlmBytesSent, metrics.TlmEncodedBytesSent = bytesSent, encodedBytesSent })
	telemetryMock := fxutil.Test[telemetry.Component](t, mocktelemetry.Module())
	metrics.TlmBytesSent = telemetryMock.NewCounter("logs", "bytes_sent", []string{"emitter", "source"}, "")
	metrics.TlmEncodedBytesSent = telemetryMock.NewCounter("logs", "encoded_bytes_sent", []string{"emitter", "source", "compression_kind"}, "")
	cfg := configmock.New(t)
	server := NewTestServer(http.StatusOK, cfg)
	defer server.Stop()
	d := NewBlockingDestination(server.Endpoint, JSONContentType, client.NewDestinationMetadata("logs", "sync", "reliable", "0", ""), cfg)

	require.NoError(t, d.Send(context.Background(), &message.Payload{Encoded: []byte("payload"), UnencodedSize: 7}))

	for _, name := range []string{"bytes_sent", "encoded_bytes_sent"} {
		metric, err := telemetryMock.(telemetry.Mock).GetCountMetric("logs", name)
		require.NoError(t, err)
		require.Len(t, metric, 1, name)
		assert.Equal(t, "logs", metric[0].Tags()["source"], name)
	}
}
