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

func (e *redisEncoder) EncodeConnection(c network.ConnectionStats, builder *model.ConnectionBuilder) (uint64, map[string]struct{}) {
	if e == nil {
		return 0, nil
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return 0, nil
	}

	staticTags := uint64(0)
	builder.SetDatabaseAggregations(func(b *bytes.Buffer) {
		staticTags |= e.encodeData(connectionData, b)
	})
	return staticTags, nil
}

func (e *redisEncoder) encodeData(connectionData *USMConnectionData[redis.Key, *redis.RequestStats], w io.Writer) uint64 {
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
				default:
					aggregationBuilder.SetCommand(uint64(model.RedisCommand_RedisUnknownCommand))
				}
				aggregationBuilder.SetTruncated(key.Truncated)
				aggregationBuilder.SetKeyName(key.KeyName.Get())

				for err, stats := range errorToStats.ErrorToStats {
					if stats.Count == 0 {
						continue
					}
					staticTags |= stats.StaticTags
					aggregationBuilder.AddErrorToStats(func(errorToStatsBuilder *model.RedisStats_ErrorToStatsEntryBuilder) {
						errorToStatsBuilder.SetKey(mapRedisErrorType(err))
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

func mapRedisErrorType(err redis.ErrorType) int32 {
	switch err {
	case redis.NoErr:
		return int32(model.RedisErrorType_RedisNoError)
	case redis.UnknownErr:
		return int32(model.RedisErrorType_RedisErrorTypeUnknown)
	case redis.Err:
		return int32(model.RedisErrorType_RedisErrErr)
	case redis.WrongType:
		return int32(model.RedisErrorType_RedisErrWrongType)
	case redis.NoAuth:
		return int32(model.RedisErrorType_RedisErrNoAuth)
	case redis.NoPerm:
		return int32(model.RedisErrorType_RedisErrNoPerm)
	case redis.Busy:
		return int32(model.RedisErrorType_RedisErrBusy)
	case redis.NoScript:
		return int32(model.RedisErrorType_RedisErrNoScript)
	case redis.Loading:
		return int32(model.RedisErrorType_RedisErrLoading)
	case redis.ReadOnly:
		return int32(model.RedisErrorType_RedisErrReadOnly)
	case redis.ExecAbort:
		return int32(model.RedisErrorType_RedisErrExecAbort)
	case redis.MasterDown:
		return int32(model.RedisErrorType_RedisErrMasterDown)
	case redis.Misconf:
		return int32(model.RedisErrorType_RedisErrMisconf)
	case redis.CrossSlot:
		return int32(model.RedisErrorType_RedisErrCrossSlot)
	case redis.TryAgain:
		return int32(model.RedisErrorType_RedisErrTryAgain)
	case redis.Ask:
		return int32(model.RedisErrorType_RedisErrAsk)
	case redis.Moved:
		return int32(model.RedisErrorType_RedisErrMoved)
	case redis.ClusterDown:
		return int32(model.RedisErrorType_RedisErrClusterDown)
	case redis.NoReplicas:
		return int32(model.RedisErrorType_RedisErrNoReplicas)
	case redis.Oom:
		return int32(model.RedisErrorType_RedisErrOom)
	case redis.NoQuorum:
		return int32(model.RedisErrorType_RedisErrNoQuorum)
	case redis.BusyKey:
		return int32(model.RedisErrorType_RedisErrBusyKey)
	case redis.Unblocked:
		return int32(model.RedisErrorType_RedisErrUnblocked)
	case redis.WrongPass:
		return int32(model.RedisErrorType_RedisErrWrongPass)
	case redis.InvalidObj:
		return int32(model.RedisErrorType_RedisErrInvalidObj)
	default:
		return int32(model.RedisErrorType_RedisErrorTypeUnknown)
	}
}
