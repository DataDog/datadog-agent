// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

package marshal

import (
	"bytes"
	"io"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

type redisEncoder struct {
	redisAggregationsBuilder *model.DatabaseAggregationsBuilder
	byConnection             *USMConnectionIndex[redis.Key, *redis.RequestStats]
}

func newRedisEncoder(redisPayloads map[redis.Key]*redis.RequestStats) *redisEncoder {
	if len(redisPayloads) == 0 {
		return nil
	}

	return &redisEncoder{
		redisAggregationsBuilder: model.NewDatabaseAggregationsBuilder(nil),
		byConnection: GroupByConnection("redis", redisPayloads, func(key redis.Key) types.ConnectionKey {
			return key.ConnectionKey
		}),
	}
}

func (e *redisEncoder) EncodeConnection(c network.ConnectionStats, builder *model.ConnectionBuilder) (staticTags uint64, dynamicTags map[string]struct{}) {
	builder.SetDatabaseAggregations(func(b *bytes.Buffer) {
		staticTags = e.encodeData(c, b)
	})
	return
}

func (e *redisEncoder) encodeData(c network.ConnectionStats, w io.Writer) uint64 {
	if e == nil {
		return 0
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return 0
	}

	var staticTags uint64
	e.redisAggregationsBuilder.Reset(w)

	for _, kv := range connectionData.Data {
		key := kv.Key
		errorToStats := kv.Value
		e.redisAggregationsBuilder.AddAggregations(func(builder *model.DatabaseStatsBuilder) {
			builder.SetRedis(func(aggregationBuilder *model.RedisStatsBuilder) {
				switch key.Command {
				case redis.GetCommand:
					aggregationBuilder.SetCommand(uint64(model.RedisCommand_RedisGetCommand))
				case redis.SetCommand:
					aggregationBuilder.SetCommand(uint64(model.RedisCommand_RedisSetCommand))
				case redis.PingCommand:
					aggregationBuilder.SetCommand(uint64(model.RedisCommand_RedisPingCommand))
				default:
					aggregationBuilder.SetCommand(uint64(model.RedisCommand_RedisUnknownCommand))
				}
				aggregationBuilder.SetTruncated(key.Truncated)
				if key.KeyName != nil {
					aggregationBuilder.SetKeyName(key.KeyName.Get())
				}

				for isErr, stats := range errorToStats.ErrorToStats {
					if stats.Count == 0 {
						continue
					}
					staticTags |= stats.StaticTags
					aggregationBuilder.AddErrorToStats(func(errorToStatsBuilder *model.RedisStats_ErrorToStatsEntryBuilder) {
						if !isErr {
							errorToStatsBuilder.SetKey(int32(model.RedisErrorType_RedisNoError))
						} else {
							errorToStatsBuilder.SetKey(int32(model.RedisErrorType_RedisErrorTypeUnknown))
						}
						errorToStatsBuilder.SetValue(func(statsBuilder *model.RedisStatsEntryBuilder) {
							statsBuilder.SetCount(uint32(stats.Count))
							if latencies := stats.Latencies; latencies != nil {
								statsBuilder.SetLatencies(func(b *bytes.Buffer) {
									latencies.EncodeProto(b)
								})
							} else {
								statsBuilder.SetFirstLatencySample(stats.FirstLatencySample)
							}
						})
					})
				}
			})
		})
	}

	return staticTags
}

func (e *redisEncoder) Close() {
	if e == nil {
		return
	}

	e.byConnection.Close()
}
