// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAgentDiscoveryAggregator(t *testing.T) {
	t.Run("parseAgentDiscoveryPayload should return empty array on empty data", func(t *testing.T) {
		payloads, err := ParseAgentDiscoveryPayload(api.Payload{Data: []byte(""), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, payloads)
	})

	t.Run("parseAgentDiscoveryPayload should ignore empty JSON connectivity probe", func(t *testing.T) {
		payloads, err := ParseAgentDiscoveryPayload(api.Payload{
			Data:        []byte("{}"),
			Encoding:    encodingJSON,
			ContentType: encodingJSON,
		})
		assert.NoError(t, err)
		assert.Empty(t, payloads)
	})

	t.Run("parseAgentDiscoveryPayload should return empty array on empty batch", func(t *testing.T) {
		payloads, err := ParseAgentDiscoveryPayload(api.Payload{Data: newAgentDiscoveryPayloadData(t), Encoding: encodingEmpty})
		assert.NoError(t, err)
		assert.Empty(t, payloads)
	})

	t.Run("parseAgentDiscoveryPayload should parse valid batch", func(t *testing.T) {
		collectedTime := time.Now()
		ingestionTime := time.Unix(1_723_456_789, 123_000_000).UTC()
		payloads, err := ParseAgentDiscoveryPayload(api.Payload{
			Data: newAgentDiscoveryPayloadData(t, &agentdiscovery.AgentDiscoveryPayload{
				Integration:        "redisdb",
				Runtime:            "docker",
				RuntimeId:          "abc123",
				IngestionTimestamp: timestamppb.New(ingestionTime),
				ConfigFiles: []*agentdiscovery.AgentDiscoveryConfigFile{
					{
						Path:          "/usr/local/etc/redis/redis.conf",
						Content:       []byte("port 6379\n"),
						PayloadFormat: agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_REDIS_CONF,
					},
				},
				EnvVars: []*agentdiscovery.AgentDiscoveryEnvVar{
					{Name: "REDIS_PORT", Value: "6379"},
				},
			}),
			Encoding:  encodingEmpty,
			Timestamp: collectedTime,
		})
		require.NoError(t, err)
		require.Len(t, payloads, 1)

		payload := payloads[0]
		assert.Equal(t, "redisdb:docker:abc123", payload.name())
		assert.Equal(t, collectedTime, payload.GetCollectedTime())
		assert.Equal(t, "redisdb", payload.Integration)
		assert.Equal(t, "docker", payload.Runtime)
		assert.Equal(t, "test-host", payload.HostID)
		assert.Equal(t, "abc123", payload.RuntimeID)
		assert.Equal(t, ingestionTime, payload.IngestionTimestamp)
		require.Len(t, payload.ConfigFiles, 1)
		assert.Equal(t, AgentDiscoveryConfigFile{
			Path:          "/usr/local/etc/redis/redis.conf",
			Content:       []byte("port 6379\n"),
			Truncated:     false,
			PayloadFormat: agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_REDIS_CONF,
		}, payload.ConfigFiles[0])
		require.Len(t, payload.EnvVars, 1)
		assert.Equal(t, AgentDiscoveryEnvVar{Name: "REDIS_PORT", Value: "6379"}, payload.EnvVars[0])
	})

	t.Run("parseAgentDiscoveryPayload should parse valid single payload", func(t *testing.T) {
		payloads, err := ParseAgentDiscoveryPayload(api.Payload{
			Data: newAgentDiscoveryPayloadData(t, &agentdiscovery.AgentDiscoveryPayload{
				Integration: "redisdb",
				Runtime:     "docker",
			}),
			Encoding: encodingEmpty,
		})
		require.NoError(t, err)
		require.Len(t, payloads, 1)
		assert.Equal(t, "redisdb", payloads[0].Integration)
	})
}

func newAgentDiscoveryPayloadData(t *testing.T, payloads ...*agentdiscovery.AgentDiscoveryPayload) []byte {
	t.Helper()

	data, err := proto.Marshal(&agentdiscovery.AgentDiscoveryPayloadBatch{
		Payloads: payloads,
		HostId:   "test-host",
	})
	require.NoError(t, err)
	return data
}
