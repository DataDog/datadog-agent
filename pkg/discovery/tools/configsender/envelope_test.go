// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildEnvelopeMatchesWireShape(t *testing.T) {
	raw := []byte("bind 127.0.0.1\nport 6379\n")

	body, hash, err := buildEnvelope("host-7", "redis", "app_native", "/etc/redis/redis.conf", "redis_conf", raw)
	require.NoError(t, err)

	expected := sha256.Sum256(raw)
	assert.Equal(t, hex.EncodeToString(expected[:]), hash)

	var got envelope
	require.NoError(t, json.Unmarshal(body, &got))

	assert.Equal(t, "demoalpha-worker", got.Service)
	assert.Equal(t, "config-ingestion-poc", got.Project)
	assert.Equal(t, "config-ingestion-poc,source:agent,host:host-7", got.Tags)
	assert.Equal(t, "redis app_native config from host-7 (/etc/redis/redis.conf)", got.Message)
	assert.Equal(t, "host-7", got.Data.HostID)
	assert.Equal(t, "redis", got.Data.Integration)
	assert.Equal(t, "app_native", got.Data.ConfigSource)
	assert.Equal(t, "/etc/redis/redis.conf", got.Data.Filename)
	assert.Equal(t, "redis_conf", got.Data.ContentType)
	assert.Equal(t, string(raw), got.Data.Raw)
}

func TestBuildEnvelopeRawPreservedExactly(t *testing.T) {
	raw := []byte("bind 127.0.0.1  \r\nport\t6379\n\n# trailing\n")

	body, _, err := buildEnvelope("h", "redis", "app_native", "/etc/redis/redis.conf", "redis_conf", raw)
	require.NoError(t, err)

	var got envelope
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, string(raw), got.Data.Raw)
}

func TestBuildEnvelopeJSONFieldNames(t *testing.T) {
	body, _, err := buildEnvelope("h", "redis", "app_native", "/etc/redis/redis.conf", "redis_conf", []byte("x"))
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &top))
	for _, key := range []string{"service", "project", "ddtags", "message", "data"} {
		_, ok := top[key]
		assert.True(t, ok, "top-level key %q missing", key)
	}

	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["data"], &data))
	for _, key := range []string{"host_id", "integration", "config_source", "filename", "content_type", "raw"} {
		_, ok := data[key]
		assert.True(t, ok, "data key %q missing", key)
	}
}
