// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"context"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/require"
)

type staticFetcher struct {
	response *pbgo.ClientGetConfigsResponse
}

func (f *staticFetcher) ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	return f.response, nil
}

func TestStatusListenerRunsWhenNoConfigChanged(t *testing.T) {
	fetcher := &staticFetcher{response: &pbgo.ClientGetConfigsResponse{
		ConfigStatus: pbgo.ConfigStatus_CONFIG_STATUS_EXPIRED,
	}}
	client, err := NewClient(fetcher, WithoutTufVerification())
	require.NoError(t, err)

	called := false
	client.SubscribeAll(state.ProductActionPlatformRunnerKeys, NewStatusListener(
		func(configs map[string]state.RawConfig, status pbgo.ConfigStatus, _ func(string, state.ApplyStatus)) {
			called = true
			require.Empty(t, configs)
			require.Equal(t, pbgo.ConfigStatus_CONFIG_STATUS_EXPIRED, status)
		},
		nil,
	))

	require.NoError(t, client.update())
	require.True(t, called)
}
