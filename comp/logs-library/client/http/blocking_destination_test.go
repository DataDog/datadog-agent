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

	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
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
			d := NewBlockingDestination(server.Endpoint, JSONContentType, cfg)

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
	d := NewBlockingDestination(server.Endpoint, JSONContentType, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := d.Send(ctx, &message.Payload{Encoded: []byte("payload")})

	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, responses, "a cancelled request must not reach the intake")
}
