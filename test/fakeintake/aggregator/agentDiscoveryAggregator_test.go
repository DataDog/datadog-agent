// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentDiscoveryAggregator(t *testing.T) {
	t.Run("parseAgentDiscoveryPayload should return empty array on empty data", func(t *testing.T) {
		payloads, err := ParseAgentDiscoveryPayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, payloads)
	})

	t.Run("parseAgentDiscoveryPayload should return empty array on empty json object", func(t *testing.T) {
		payloads, err := ParseAgentDiscoveryPayload(api.Payload{Data: []byte("{}"), Encoding: encodingJSON})
		assert.NoError(t, err)
		assert.Empty(t, payloads)
	})

	t.Run("parseAgentDiscoveryPayload should parse valid batch", func(t *testing.T) {
		collectedTime := time.Now()
		payloads, err := ParseAgentDiscoveryPayload(api.Payload{
			Data:      []byte(`[{"integration":"redisdb","service_id":"docker://abc123","runtime":"docker","configs":[{"type":"file","path":"/usr/local/etc/redis/redis.conf","content_base64":"cG9ydCA2Mzc5Cg==","truncated":false}]}]`),
			Encoding:  encodingJSON,
			Timestamp: collectedTime,
		})
		require.NoError(t, err)
		require.Len(t, payloads, 1)

		payload := payloads[0]
		assert.Equal(t, "redisdb:docker://abc123", payload.name())
		assert.Equal(t, collectedTime, payload.GetCollectedTime())
		assert.Equal(t, "redisdb", payload.Integration)
		assert.Equal(t, "docker://abc123", payload.ServiceID)
		assert.Equal(t, "docker", payload.Runtime)
		require.Len(t, payload.Configs, 1)
		assert.Equal(t, AgentDiscoveryConfig{
			Type:          "file",
			Path:          "/usr/local/etc/redis/redis.conf",
			ContentBase64: "cG9ydCA2Mzc5Cg==",
			Truncated:     false,
		}, payload.Configs[0])
		assert.Contains(t, payload.RawPayload, "integration")
		assert.NotContains(t, payload.RawPayload, "agent_version")
	})

	t.Run("parseAgentDiscoveryPayload should parse valid single payload", func(t *testing.T) {
		payloads, err := ParseAgentDiscoveryPayload(api.Payload{
			Data:     []byte(`{"integration":"redisdb","service_id":"docker://abc123","runtime":"docker","configs":[]}`),
			Encoding: encodingJSON,
		})
		require.NoError(t, err)
		require.Len(t, payloads, 1)
		assert.Equal(t, "redisdb", payloads[0].Integration)
	})
}
