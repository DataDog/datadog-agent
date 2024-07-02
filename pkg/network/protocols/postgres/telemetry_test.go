// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

type telemetryResults struct {
	queryLength               [bucketLength]int64
	failedTableNameExtraction int64
	failedOperationExtraction int64
}

func Test_getBucketIndex(t *testing.T) {
	for i := 0; i <= 130; i++ {
		require.Equal(t, 0, getBucketIndex(i))
	}
	for i := 131; i <= 145; i++ {
		require.Equal(t, 1, getBucketIndex(i))
	}
	for i := 146; i <= 160; i++ {
		require.Equal(t, 2, getBucketIndex(i))
	}
	for i := 161; i <= 175; i++ {
		require.Equal(t, 3, getBucketIndex(i))
	}
	for i := 176; i <= 190; i++ {
		require.Equal(t, 4, getBucketIndex(i))
	}
	for i := 191; i <= 205; i++ {
		require.Equal(t, 5, getBucketIndex(i))
	}
	for i := 206; i <= 220; i++ {
		require.Equal(t, 6, getBucketIndex(i))
	}
	for i := 221; i <= 235; i++ {
		require.Equal(t, 7, getBucketIndex(i))
	}
	for i := 236; i <= 250; i++ {
		require.Equal(t, 8, getBucketIndex(i))
	}
	for i := 251; i <= 300; i++ {
		require.Equal(t, 9, getBucketIndex(i))
	}
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
						Original_query_size: 100,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 131,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 160,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 161,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 176,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 191,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 206,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 221,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 236,
					},
				},
				{
					Tx: EbpfTx{
						Original_query_size: 251,
					},
				},
			},

			expectedTelemetry: telemetryResults{
				queryLength:               [bucketLength]int64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
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
				queryLength:               [bucketLength]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			},
		},
		{
			name:  "failed table name extraction",
			tx:    []*EbpfEvent{{}},
			query: "CREATE TABLE",
			expectedTelemetry: telemetryResults{
				failedTableNameExtraction: 1,
				queryLength:               [bucketLength]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			},
		},
		{
			name:  "failed table name and operation extraction",
			tx:    []*EbpfEvent{{}},
			query: "CRE TABLE",
			expectedTelemetry: telemetryResults{
				failedTableNameExtraction: 1,
				failedOperationExtraction: 1,
				queryLength:               [bucketLength]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
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
