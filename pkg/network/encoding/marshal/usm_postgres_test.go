// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	postgresClientPort = uint16(2345)
	postgresServerPort = uint16(5432)
	tableName          = "TableName"
)

var (
	postgresDefaultConnection = network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Source: localhost,
		Dest:   localhost,
		SPort:  postgresClientPort,
		DPort:  postgresServerPort,
	}}
)

type PostgresSuite struct {
	suite.Suite
}

func TestPostgresStats(t *testing.T) {
	skipIfNotLinux(t)
	suite.Run(t, &PostgresSuite{})
}

func (s *PostgresSuite) TestFormatPostgresStats() {
	t := s.T()

	selectKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.SelectOP,
		tableName,
	)
	insertKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.InsertOP,
		tableName,
	)
	updateKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.UpdateOP,
		tableName,
	)
	createKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.CreateTableOP,
		tableName,
	)
	dropKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.DropTableOP,
		tableName,
	)
	deleteKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.DeleteTableOP,
		tableName,
	)
	alterKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.AlterTableOP,
		tableName,
	)
	truncateKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.TruncateTableOP,
		tableName,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				postgresDefaultConnection,
			},
		},
		Postgres: map[postgres.Key]*postgres.RequestStat{
			selectKey: {
				Count:              10,
				FirstLatencySample: 5,
			},
			insertKey: {
				Count:              10,
				FirstLatencySample: 5,
			},
			deleteKey: {
				Count:              10,
				FirstLatencySample: 5,
			},
			truncateKey: {
				Count:              10,
				FirstLatencySample: 5,
			},
			updateKey: {
				Count:              10,
				FirstLatencySample: 5,
			},
			createKey: {
				Count:              10,
				FirstLatencySample: 5,
			},
			dropKey: {
				Count:              10,
				FirstLatencySample: 5,
			},
			alterKey: {
				Count:              10,
				FirstLatencySample: 5,
			},
		},
	}
	out := &model.DatabaseAggregations{
		Aggregations: []*model.DatabaseStats{
			{
				DbStats: &model.DatabaseStats_Postgres{
					Postgres: &model.PostgresStats{
						TableName:          tableName,
						Operation:          model.PostgresOperation_PostgresSelectOp,
						FirstLatencySample: 5,
						Count:              10,
					},
				},
			},
			{
				DbStats: &model.DatabaseStats_Postgres{
					Postgres: &model.PostgresStats{
						TableName:          tableName,
						Operation:          model.PostgresOperation_PostgresInsertOp,
						FirstLatencySample: 5,
						Count:              10,
					},
				},
			},
			{
				DbStats: &model.DatabaseStats_Postgres{
					Postgres: &model.PostgresStats{
						TableName:          tableName,
						Operation:          model.PostgresOperation_PostgresUpdateOp,
						FirstLatencySample: 5,
						Count:              10,
					},
				},
			},
			{
				DbStats: &model.DatabaseStats_Postgres{
					Postgres: &model.PostgresStats{
						TableName:          tableName,
						Operation:          model.PostgresOperation_PostgresCreateOp,
						FirstLatencySample: 5,
						Count:              10,
					},
				},
			},
			{
				DbStats: &model.DatabaseStats_Postgres{
					Postgres: &model.PostgresStats{
						TableName:          tableName,
						Operation:          model.PostgresOperation_PostgresDropOp,
						FirstLatencySample: 5,
						Count:              10,
					},
				},
			},
			{
				DbStats: &model.DatabaseStats_Postgres{
					Postgres: &model.PostgresStats{
						TableName:          tableName,
						Operation:          model.PostgresOperation_PostgresDeleteOp,
						FirstLatencySample: 5,
						Count:              10,
					},
				},
			},
			{
				DbStats: &model.DatabaseStats_Postgres{
					Postgres: &model.PostgresStats{
						TableName:          tableName,
						Operation:          model.PostgresOperation_PostgresAlterOp,
						FirstLatencySample: 5,
						Count:              10,
					},
				},
			},
			{
				DbStats: &model.DatabaseStats_Postgres{
					Postgres: &model.PostgresStats{
						TableName:          tableName,
						Operation:          model.PostgresOperation_PostgresTruncateOp,
						FirstLatencySample: 5,
						Count:              10,
					},
				},
			},
		},
	}

	encoder := newPostgresEncoder(in.Postgres)
	t.Cleanup(encoder.Close)

	aggregations := getPostgresAggregations(t, encoder, in.Conns[0])

	require.NotNil(t, aggregations)
	assert.ElementsMatch(t, out.Aggregations, aggregations.Aggregations)
}

func (s *PostgresSuite) TestPostgresIDCollisionRegression() {
	t := s.T()
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  postgresClientPort,
			Dest:   localhost,
			DPort:  postgresServerPort,
			Pid:    1,
		}},
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  postgresClientPort,
			Dest:   localhost,
			DPort:  postgresServerPort,
			Pid:    2,
		}},
	}

	postgresKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.SelectOP,
		tableName,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		Postgres: map[postgres.Key]*postgres.RequestStat{
			postgresKey: {
				Count:              10,
				FirstLatencySample: 3,
			},
		},
	}

	encoder := newPostgresEncoder(in.Postgres)
	t.Cleanup(encoder.Close)
	aggregations := getPostgresAggregations(t, encoder, in.Conns[0])

	// assert that the first connection matching the Postgres data will get back a non-nil result
	assert.Equal(tableName, aggregations.Aggregations[0].GetPostgres().GetTableName())
	assert.Equal(uint32(10), aggregations.Aggregations[0].GetPostgres().GetCount())
	assert.Equal(float64(3), aggregations.Aggregations[0].GetPostgres().GetFirstLatencySample())

	// assert that the other connections sharing the same (source,destination)
	// addresses but different PIDs *won't* be associated with the Postgres stats
	// object
	streamer := NewProtoTestStreamer[*model.Connection]()
	encoder.WritePostgresAggregations(in.Conns[1], model.NewConnectionBuilder(streamer))
	var conn model.Connection
	streamer.Unwrap(t, &conn)
	assert.Empty(conn.DataStreamsAggregations)
}

func (s *PostgresSuite) TestPostgresLocalhostScenario() {
	t := s.T()
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  postgresClientPort,
			Dest:   localhost,
			DPort:  postgresServerPort,
			Pid:    1,
		}},
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  postgresServerPort,
			Dest:   localhost,
			DPort:  postgresClientPort,
			Pid:    2,
		}},
	}

	postgresKey := postgres.NewKey(
		localhost,
		localhost,
		postgresClientPort,
		postgresServerPort,
		postgres.InsertOP,
		tableName,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		Postgres: map[postgres.Key]*postgres.RequestStat{
			postgresKey: {
				Count:              10,
				FirstLatencySample: 2,
			},
		},
	}

	encoder := newPostgresEncoder(in.Postgres)
	t.Cleanup(encoder.Close)

	// assert that both ends (client:server, server:client) of the connection
	// will have Postgres stats
	for _, conn := range in.Conns {
		aggregations := getPostgresAggregations(t, encoder, conn)
		assert.Equal(tableName, aggregations.Aggregations[0].GetPostgres().GetTableName())
		assert.Equal(uint32(10), aggregations.Aggregations[0].GetPostgres().GetCount())
		assert.Equal(float64(2), aggregations.Aggregations[0].GetPostgres().GetFirstLatencySample())
	}
}

func getPostgresAggregations(t *testing.T, encoder *postgresEncoder, c network.ConnectionStats) *model.DatabaseAggregations {
	streamer := NewProtoTestStreamer[*model.Connection]()
	encoder.WritePostgresAggregations(c, model.NewConnectionBuilder(streamer))

	var conn model.Connection
	streamer.Unwrap(t, &conn)

	var aggregations model.DatabaseAggregations
	err := proto.Unmarshal(conn.DatabaseAggregations, &aggregations)
	require.NoError(t, err)

	return &aggregations
}

func generateBenchMarkPayloadPostgres(sourcePortsMax, destPortsMax uint16) network.Connections {
	localhost := util.AddressFromString("127.0.0.1")

	payload := network.Connections{
		BufferedData: network.BufferedData{
			Conns: make([]network.ConnectionStats, sourcePortsMax*destPortsMax),
		},
		Postgres: make(map[postgres.Key]*postgres.RequestStat),
	}

	for sport := uint16(0); sport < sourcePortsMax; sport++ {
		for dport := uint16(0); dport < destPortsMax; dport++ {
			index := sport*sourcePortsMax + dport

			payload.Conns[index].Dest = localhost
			payload.Conns[index].Source = localhost
			payload.Conns[index].DPort = dport + 1
			payload.Conns[index].SPort = sport + 1
			if index%2 == 0 {
				payload.Conns[index].IPTranslation = &network.IPTranslation{
					ReplSrcIP:   localhost,
					ReplDstIP:   localhost,
					ReplSrcPort: dport + 1,
					ReplDstPort: sport + 1,
				}
			}

			payload.Postgres[postgres.NewKey(
				localhost,
				localhost,
				sport+1,
				dport+1,
				postgres.SelectOP,
				tableName,
			)] = &postgres.RequestStat{
				Count:              10,
				FirstLatencySample: 5,
			}
		}
	}

	return payload
}

func commonBenchmarkPostgresEncoder(b *testing.B, numberOfPorts uint16) {
	payload := generateBenchMarkPayloadPostgres(numberOfPorts, numberOfPorts)
	streamer := NewProtoTestStreamer[*model.Connection]()
	a := model.NewConnectionBuilder(streamer)
	b.ResetTimer()
	b.ReportAllocs()
	var h *postgresEncoder
	for i := 0; i < b.N; i++ {
		h = newPostgresEncoder(payload.Postgres)
		streamer.Reset()
		for _, conn := range payload.Conns {
			h.WritePostgresAggregations(conn, a)
		}
		h.Close()
	}
}

func BenchmarkPostgresEncoder100Requests(b *testing.B) {
	commonBenchmarkPostgresEncoder(b, 10)
}

func BenchmarkPostgresEncoder10000Requests(b *testing.B) {
	commonBenchmarkPostgresEncoder(b, 100)
}
