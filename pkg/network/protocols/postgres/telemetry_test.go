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
	// We want to validate that the bucket index is calculated correctly, given the BufferSize and the bucketLength.
	testCases := []struct {
		start, end, expected int
	}{
		{0, BufferSize - 2*bucketLength, 0},
		{BufferSize - 2*bucketLength + 1, BufferSize - bucketLength, 1},
		{BufferSize - bucketLength + 1, BufferSize, 2},
		{BufferSize + 1, BufferSize + bucketLength, 3},
		{BufferSize + bucketLength + 1, BufferSize + 2*bucketLength, 4},
		{BufferSize + 2*bucketLength + 1, BufferSize + 3*bucketLength, 5},
		{BufferSize + 3*bucketLength + 1, BufferSize + 4*bucketLength, 6},
		{BufferSize + 4*bucketLength + 1, BufferSize + 5*bucketLength, 7},
		{BufferSize + 5*bucketLength + 1, BufferSize + 6*bucketLength, 8},
		{BufferSize + 6*bucketLength + 1, BufferSize + 7*bucketLength, 9},
	}

	for _, tc := range testCases {
		for i := tc.start; i <= tc.end; i++ {
			require.Equal(t, tc.expected, getBucketIndex(i), "query length %d should be in bucket %d", i, tc.expected)
		}
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
				createEbpfEvent(BufferSize - 2*bucketLength),
				createEbpfEvent(BufferSize - bucketLength),
				createEbpfEvent(BufferSize),
				createEbpfEvent(BufferSize + 1),
				createEbpfEvent(BufferSize + bucketLength + 1),
				createEbpfEvent(BufferSize + 2*bucketLength + 1),
				createEbpfEvent(BufferSize + 3*bucketLength + 1),
				createEbpfEvent(BufferSize + 4*bucketLength + 1),
				createEbpfEvent(BufferSize + 5*bucketLength + 1),
				createEbpfEvent(BufferSize + 6*bucketLength + 1),
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

func createEbpfEvent(querySize int) *EbpfEvent {
	return &EbpfEvent{
		Tx: EbpfTx{
			Original_query_size: uint32(querySize),
		},
	}
}

func verifyTelemetry(t *testing.T, tel *Telemetry, expected telemetryResults) {
	for i := 0; i < len(tel.queryLengthBuckets); i++ {
		assert.Equal(t, expected.queryLength[i], tel.queryLengthBuckets[i].Get(), "queryLength for bucket %d count is incorrect", i)
	}
	assert.Equal(t, expected.failedTableNameExtraction, tel.failedTableNameExtraction.Get(), "failedTableNameExtraction count is incorrect")
	assert.Equal(t, expected.failedOperationExtraction, tel.failedOperationExtraction.Get(), "failedOperationExtraction count is incorrect")
}
