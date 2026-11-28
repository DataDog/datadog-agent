// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package daemon

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type unusedFetcher struct{}

func (unusedFetcher) ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	return nil, errors.New("not used")
}

func TestNewRemoteConfigUsesTheClientID(t *testing.T) {
	id := client.GenerateClientID()

	first, err := newRemoteConfig(unusedFetcher{}, id)
	require.NoError(t, err)
	second, err := newRemoteConfig(unusedFetcher{}, id)
	require.NoError(t, err)

	assert.Equal(t, id, first.client.GetClientID())
	assert.Equal(t, id, second.client.GetClientID(), "daemons that replace one another stay the same client")
}

func TestNewRemoteConfigGivesARandomIDWithoutOne(t *testing.T) {
	first, err := newRemoteConfig(unusedFetcher{}, "")
	require.NoError(t, err)
	second, err := newRemoteConfig(unusedFetcher{}, "")
	require.NoError(t, err)

	assert.True(t, client.ValidClientID(first.client.GetClientID()))
	assert.NotEqual(t, first.client.GetClientID(), second.client.GetClientID())
}
