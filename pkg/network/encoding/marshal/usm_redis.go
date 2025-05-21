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
				aggregationBuilder.SetKeyName(key.KeyName)

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

func mapRedisErrorType(err redis.RedisErrorType) int32 {
	switch err {
	case redis.RedisNoErr:
		return int32(model.RedisErrorType_RedisNoError)
	case redis.RedisErrUnknown:
		return int32(model.RedisErrorType_RedisErrorTypeUnknown)
	case redis.RedisErrErr:
		return int32(model.RedisErrorType_RedisErrErr)
	case redis.RedisErrWrongType:
		return int32(model.RedisErrorType_RedisErrWrongType)
	case redis.RedisErrNoAuth:
		return int32(model.RedisErrorType_RedisErrNoAuth)
	case redis.RedisErrNoPerm:
		return int32(model.RedisErrorType_RedisErrNoPerm)
	case redis.RedisErrBusy:
		return int32(model.RedisErrorType_RedisErrBusy)
	case redis.RedisErrNoScript:
		return int32(model.RedisErrorType_RedisErrNoScript)
	case redis.RedisErrLoading:
		return int32(model.RedisErrorType_RedisErrLoading)
	case redis.RedisErrReadOnly:
		return int32(model.RedisErrorType_RedisErrReadOnly)
	case redis.RedisErrExecAbort:
		return int32(model.RedisErrorType_RedisErrExecAbort)
	case redis.RedisErrMasterDown:
		return int32(model.RedisErrorType_RedisErrMasterDown)
	case redis.RedisErrMisconf:
		return int32(model.RedisErrorType_RedisErrMisconf)
	case redis.RedisErrCrossSlot:
		return int32(model.RedisErrorType_RedisErrCrossSlot)
	case redis.RedisErrTryAgain:
		return int32(model.RedisErrorType_RedisErrTryAgain)
	case redis.RedisErrAsk:
		return int32(model.RedisErrorType_RedisErrAsk)
	case redis.RedisErrMoved:
		return int32(model.RedisErrorType_RedisErrMoved)
	case redis.RedisErrClusterDown:
		return int32(model.RedisErrorType_RedisErrClusterDown)
	case redis.RedisErrNoReplicas:
		return int32(model.RedisErrorType_RedisErrNoReplicas)
	case redis.RedisErrOom:
		return int32(model.RedisErrorType_RedisErrOom)
	case redis.RedisErrNoQuorum:
		return int32(model.RedisErrorType_RedisErrNoQuorum)
	case redis.RedisErrBusyKey:
		return int32(model.RedisErrorType_RedisErrBusyKey)
	case redis.RedisErrUnblocked:
		return int32(model.RedisErrorType_RedisErrUnblocked)
	case redis.RedisErrUnsupported:
		return int32(model.RedisErrorType_RedisErrUnsupported)
	case redis.RedisErrSyntax:
		return int32(model.RedisErrorType_RedisErrSyntax)
	case redis.RedisErrClientClosed:
		return int32(model.RedisErrorType_RedisErrClientClosed)
	case redis.RedisErrProxy:
		return int32(model.RedisErrorType_RedisErrProxy)
	case redis.RedisErrWrongPass:
		return int32(model.RedisErrorType_RedisErrWrongPass)
	case redis.RedisErrInvalid:
		return int32(model.RedisErrorType_RedisErrInvalid)
	case redis.RedisErrDeprecated:
		return int32(model.RedisErrorType_RedisErrDeprecated)
	default:
		return int32(model.RedisErrorType_RedisErrorTypeUnknown)
	}
}
