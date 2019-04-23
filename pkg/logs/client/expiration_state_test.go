// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsExpiredReturnsTrueWhenTimeElapsed(t *testing.T) {
	expirationState := NewExpirationState(10*time.Millisecond, 1, time.Millisecond)
	assert.False(t, expirationState.IsExpired())

	expirationState.Reset()
	assert.False(t, expirationState.IsExpired())

	time.Sleep(20 * time.Millisecond)
	assert.True(t, expirationState.IsExpired())
}

func TestIsExpiredReturnsFalseAfterClear(t *testing.T) {
	expirationState := NewExpirationState(10*time.Millisecond, 1, time.Millisecond)
	expirationState.Reset()
	assert.False(t, expirationState.IsExpired())

	time.Sleep(20 * time.Millisecond)
	assert.True(t, expirationState.IsExpired())

	expirationState.Clear()
	assert.False(t, expirationState.IsExpired())
}
