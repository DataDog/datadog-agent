// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAgentKey(t *testing.T) {
	assert.Equal(t, "agent", BuildAgentKey())
	assert.Equal(t, "agent/foo", BuildAgentKey("foo"))
	assert.Equal(t, "agent/foo/bar", BuildAgentKey("foo", "bar"))
	assert.Equal(t, "agent/foo/bar/baz", BuildAgentKey("foo", "bar", "baz"))
}

func TestGet(t *testing.T) {
	key := "test-get-key"
	Cache.Delete(key)

	callCount := 0
	cb := func() (string, error) {
		callCount++
		return "hello", nil
	}

	// first call: cache miss, calls cb
	val, err := Get(key, cb)
	require.NoError(t, err)
	assert.Equal(t, "hello", val)
	assert.Equal(t, 1, callCount)

	// second call: cache hit, cb not called
	val, err = Get(key, cb)
	require.NoError(t, err)
	assert.Equal(t, "hello", val)
	assert.Equal(t, 1, callCount)

	Cache.Delete(key)
}

func TestGetWithExpirationError(t *testing.T) {
	key := "test-get-error-key"
	Cache.Delete(key)

	cb := func() (string, error) {
		return "", errors.New("fetch failed")
	}

	// errors are not cached
	_, err := GetWithExpiration(key, cb, time.Minute)
	assert.Error(t, err)

	// verify value was not cached
	_, found := Cache.Get(key)
	assert.False(t, found)

	Cache.Delete(key)
}
