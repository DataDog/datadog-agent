// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type noFetcher struct{}

func (noFetcher) ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	return nil, errors.New("not used")
}

func TestWithClientIDKeepsTheID(t *testing.T) {
	id := GenerateClientID()

	first, err := newClient(noFetcher{}, WithoutTufVerification(), WithClientID(id))
	require.NoError(t, err)
	second, err := newClient(noFetcher{}, WithoutTufVerification(), WithClientID(id))
	require.NoError(t, err)

	assert.Equal(t, id, first.GetClientID())
	assert.Equal(t, id, second.GetClientID(), "two clients given the same ID report it")
}

func TestClientIDIsRandomByDefault(t *testing.T) {
	first, err := newClient(noFetcher{}, WithoutTufVerification())
	require.NoError(t, err)
	second, err := newClient(noFetcher{}, WithoutTufVerification(), WithClientID(""))
	require.NoError(t, err)

	assert.True(t, ValidClientID(first.GetClientID()))
	assert.True(t, ValidClientID(second.GetClientID()), "an empty ID keeps the random default")
	assert.NotEqual(t, first.GetClientID(), second.GetClientID())
}

func TestValidClientID(t *testing.T) {
	assert.True(t, ValidClientID(GenerateClientID()))
	assert.True(t, ValidClientID(strings.Repeat("a", idSize)))
	assert.True(t, ValidClientID(strings.Repeat("_", idSize)), "the alphabet includes _ and -")
	assert.True(t, ValidClientID(strings.Repeat("-", idSize)))

	assert.False(t, ValidClientID(""))
	assert.False(t, ValidClientID(strings.Repeat("a", idSize-1)))
	assert.False(t, ValidClientID(strings.Repeat("a", idSize+1)))
	assert.False(t, ValidClientID(strings.Repeat("a", idSize-1)+"/"), "a character outside the alphabet")
	assert.False(t, ValidClientID(strings.Repeat("a", idSize-1)+"\n"), "a trailing newline")
	assert.False(t, ValidClientID(strings.Repeat("é", idSize)), "a multi-byte character")
}
