// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	byConnection             *USMConnectionIndex[redis.Key, *redis.RequestStat]
}

func newRedisEncoder(redisPayloads map[redis.Key]*redis.RequestStat) *redisEncoder {
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

func (e *redisEncoder) WriteRedisAggregations(c network.ConnectionStats, builder *model.ConnectionBuilder) uint64 {
	if e == nil {
		return 0
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return 0
	}

	staticTags := uint64(0)
	builder.SetDatabaseAggregations(func(b *bytes.Buffer) {
		staticTags |= e.encodeData(connectionData, b)
	})
	return staticTags
}

func (e *redisEncoder) encodeData(connectionData *USMConnectionData[redis.Key, *redis.RequestStat], w io.Writer) uint64 {
	var staticTags uint64
	e.redisAggregationsBuilder.Reset(w)

	for range connectionData.Data {
		e.redisAggregationsBuilder.AddAggregations(func(builder *model.DatabaseStatsBuilder) {
			builder.SetRedis(func(*model.RedisStatsBuilder) {})
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
