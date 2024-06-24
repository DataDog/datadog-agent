// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

type telemetryResults struct {
	queryLength               [bucketRange]int64
	failedTableNameExtraction int64
	failedOperationExtraction int64
}

func TestTelemetry_Count(t *testing.T) {
	tests := []struct {
		name              string
		query             string
		tx                []*EbpfEvent
		expectedTelemetry telemetryResults
	}{
		{
			name: "exceeded query length bucket for each bucket ones",
			tx: []*EbpfEvent{
				{
					Tx: EbpfTx{
						Original_query_size: 19,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 35,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 64,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 65,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 80,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 95,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 110,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 125,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 140,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 200,
					},
				},
			},

			expectedTelemetry: telemetryResults{
				queryLength:               [bucketRange]int64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
				failedOperationExtraction: 10,
				failedTableNameExtraction: 10,
			},
		},
		{
			name:  "failed operation extraction",
			tx:    []*EbpfEvent{{}},
			query: "CREA TABLE dummy",
			expectedTelemetry: telemetryResults{
				failedOperationExtraction: 1,
				queryLength:               [bucketRange]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			},
		},
		{
			name:  "failed table name extraction",
			tx:    []*EbpfEvent{{}},
			query: "CREATE TABLE",
			expectedTelemetry: telemetryResults{
				failedTableNameExtraction: 1,
				queryLength:               [bucketRange]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			},
		},
		{
			name:  "failed table name and operation extraction",
			tx:    []*EbpfEvent{{}},
			query: "CRE TABLE",
			expectedTelemetry: telemetryResults{
				failedTableNameExtraction: 1,
				failedOperationExtraction: 1,
				queryLength:               [bucketRange]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telemetry.Clear()
			tel := NewTelemetry()
			if tt.query != "" {
				tt.tx[0].Tx.Original_query_size = uint32(len(tt.query))
				copy(tt.tx[0].Tx.Request_fragment[:], tt.query)
			}
			for _, tx := range tt.tx {
				ep := NewEventWrapper(tx)
				tel.Count(tx, ep)
			}
			verifyTelemetry(t, tel, tt.expectedTelemetry)
		})
	}
}

func verifyTelemetry(t *testing.T, tel *Telemetry, expected telemetryResults) {
	for i := 0; i < len(tel.queryLengthBuckets); i++ {
		assert.Equal(t, expected.queryLength[i], tel.queryLengthBuckets[i].Get(), "queryLength for bucket %d count is incorrect", i)
	}
	assert.Equal(t, expected.failedTableNameExtraction, tel.failedTableNameExtraction.Get(), "failedTableNameExtraction count is incorrect")
	assert.Equal(t, expected.failedOperationExtraction, tel.failedOperationExtraction.Get(), "failedOperationExtraction count is incorrect")
}
