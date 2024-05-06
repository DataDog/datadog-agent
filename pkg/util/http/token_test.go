// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPIToken(t *testing.T) {
	cb := func(ctx context.Context) (string, time.Time, error) { return "test", time.Time{}, nil }

	token := NewAPIToken(cb)
	require.NotNil(t, token)

	val, _, _ := token.renewCallback(context.Background())
	assert.Equal(t, "test", val)

	assert.True(t, token.ExpirationDate.IsZero())
	assert.Equal(t, "", token.Value)
}

func TestGetNewToken(t *testing.T) {
	nbCbCall := 0
	tokenReturnValue := "test"
	expireReturnValue := time.Now().Add(10 * time.Minute)

	cb := func(ctx context.Context) (string, time.Time, error) {
		nbCbCall++
		return tokenReturnValue, expireReturnValue, nil
	}

	token := NewAPIToken(cb)
	val, err := token.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test", val)
	assert.Equal(t, expireReturnValue, token.ExpirationDate)
	assert.Equal(t, 1, nbCbCall)

	// Test expire date
	tokenReturnValue = "test2"

	val, err = token.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test", val)
	assert.Equal(t, expireReturnValue, token.ExpirationDate)
	assert.Equal(t, 1, nbCbCall)

	// expire token
	token.ExpirationDate = time.Now().Add(-10 * time.Minute)
	val, err = token.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test2", val)
	assert.Equal(t, expireReturnValue, token.ExpirationDate)
	assert.Equal(t, 2, nbCbCall)
}
