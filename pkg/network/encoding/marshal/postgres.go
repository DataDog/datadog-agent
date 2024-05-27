// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"bytes"
	"io"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

type postgresEncoder struct {
	postgresAggregationsBuilder *model.DatabaseAggregationsBuilder
	byConnection                *USMConnectionIndex[postgres.Key, *postgres.RequestStat]
}

func newPostgresEncoder(postgresPayloads map[postgres.Key]*postgres.RequestStat) *postgresEncoder {
	if len(postgresPayloads) == 0 {
		return nil
	}

	return &postgresEncoder{
		postgresAggregationsBuilder: model.NewDatabaseAggregationsBuilder(nil),
		byConnection: GroupByConnection("postgres", postgresPayloads, func(key postgres.Key) types.ConnectionKey {
			return key.ConnectionKey
		}),
	}
}

func (e *postgresEncoder) WritePostgresAggregations(c network.ConnectionStats, builder *model.ConnectionBuilder) {
	if e == nil {
		return
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return
	}

	builder.SetDatabaseAggregations(func(b *bytes.Buffer) {
		e.encodeData(connectionData, b)
	})
}

func (e *postgresEncoder) encodeData(connectionData *USMConnectionData[postgres.Key, *postgres.RequestStat], w io.Writer) {
	e.postgresAggregationsBuilder.Reset(w)

	for _, kv := range connectionData.Data {
		key := kv.Key
		stats := kv.Value
		e.postgresAggregationsBuilder.AddAggregations(func(builder *model.DatabaseStatsBuilder) {
			builder.SetPostgres(func(statsBuilder *model.PostgresStatsBuilder) {
				statsBuilder.SetTableName(key.TableName)
				statsBuilder.SetOperation(uint64(toPostgresModelOperation(key.Operation)))
				if latencies := stats.Latencies; latencies != nil {
					blob, _ := proto.Marshal(latencies.ToProto())
					statsBuilder.SetLatencies(func(b *bytes.Buffer) {
						b.Write(blob)
					})
				} else {
					statsBuilder.SetFirstLatencySample(stats.FirstLatencySample)
				}
				statsBuilder.SetCount(uint32(stats.Count))
			})
		})
	}
}

func (e *postgresEncoder) Close() {
	if e == nil {
		return
	}

	e.byConnection.Close()
}

func toPostgresModelOperation(op postgres.Operation) model.PostgresOperation {
	switch op {
	case postgres.SelectOP:
		return model.PostgresOperation_PostgresSelectOp
	case postgres.InsertOP:
		return model.PostgresOperation_PostgresInsertOp
	case postgres.UpdateOP:
		return model.PostgresOperation_PostgresUpdateOp
	case postgres.CreateTableOP:
		return model.PostgresOperation_PostgresCreateOp
	case postgres.DropTableOP:
		return model.PostgresOperation_PostgresDropOp
	case postgres.TruncateTableOP:
		return model.PostgresOperation_PostgresTruncateOp
	default:
		return model.PostgresOperation_PostgresUnknownOp
	}
}
