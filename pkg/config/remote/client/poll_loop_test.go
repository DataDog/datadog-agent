// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type mockPollFetcher struct {
	fn func(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error)
}

func (m *mockPollFetcher) ClientGetConfigs(ctx context.Context, req *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	return m.fn(ctx, req)
}

func TestPollLoopStopsWithoutErrorWhenClosedDuringUpdate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		var client *Client
		fetcher := &mockPollFetcher{
			fn: func(_ context.Context, _ *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
				calls++
				if calls == 1 {
					return &pbgo.ClientGetConfigsResponse{}, nil
				}
				client.Close()
				return nil, status.Error(codes.Canceled, "context canceled")
			},
		}

		var err error
		client, err = newClient(fetcher, WithPollInterval(time.Millisecond), func(opts *Options) {
			opts.skipTufVerification = true
		})
		require.NoError(t, err)

		client.pollLoop()

		assert.Equal(t, 2, calls)
		assert.NoError(t, client.lastUpdateError)
	})
}
