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

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/ebpf"
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
		{0, ebpf.BufferSize - 2*bucketLength, 0},
		{ebpf.BufferSize - 2*bucketLength + 1, ebpf.BufferSize - bucketLength, 1},
		{ebpf.BufferSize - bucketLength + 1, ebpf.BufferSize, 2},
		{ebpf.BufferSize + 1, ebpf.BufferSize + bucketLength, 3},
		{ebpf.BufferSize + bucketLength + 1, ebpf.BufferSize + 2*bucketLength, 4},
		{ebpf.BufferSize + 2*bucketLength + 1, ebpf.BufferSize + 3*bucketLength, 5},
		{ebpf.BufferSize + 3*bucketLength + 1, ebpf.BufferSize + 4*bucketLength, 6},
		{ebpf.BufferSize + 4*bucketLength + 1, ebpf.BufferSize + 5*bucketLength, 7},
		{ebpf.BufferSize + 5*bucketLength + 1, ebpf.BufferSize + 6*bucketLength, 8},
		{ebpf.BufferSize + 6*bucketLength + 1, ebpf.BufferSize + 7*bucketLength, 9},
	}

	cfg := config.New()
	telemetry := NewTelemetry(cfg)

	for _, tc := range testCases {
		for i := tc.start; i <= tc.end; i++ {
			require.Equal(t, tc.expected, telemetry.getBucketIndex(i), "query length %d should be in bucket %d", i, tc.expected)
		}
	}
}

// telemetryTestBufferSize serves as example configuration for the telemetry buffer size.
const telemetryTestBufferSize = 2 * ebpf.BufferSize

func TestTelemetry_Count(t *testing.T) {
	tests := []struct {
		name              string
		query             string
		maxBufferSize     int
		tx                []*ebpf.EbpfEvent
		expectedTelemetry telemetryResults
	}{
		{
			name: "exceeded query length bucket for each bucket ones",
			tx: []*ebpf.EbpfEvent{
				createEbpfEvent(ebpf.BufferSize - 2*bucketLength),
				createEbpfEvent(ebpf.BufferSize - bucketLength),
				createEbpfEvent(ebpf.BufferSize),
				createEbpfEvent(ebpf.BufferSize + 1),
				createEbpfEvent(ebpf.BufferSize + bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 2*bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 3*bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 4*bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 5*bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 6*bucketLength + 1),
			},

			expectedTelemetry: telemetryResults{
				queryLength:               [bucketLength]int64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
				failedOperationExtraction: 10,
				failedTableNameExtraction: 10,
			},
		},
		{
			name: "exceeded query length bucket for each bucket ones with telemetry config",
			tx: []*ebpf.EbpfEvent{
				createEbpfEvent(telemetryTestBufferSize - 2*bucketLength),
				createEbpfEvent(telemetryTestBufferSize - bucketLength),
				createEbpfEvent(telemetryTestBufferSize),
				createEbpfEvent(telemetryTestBufferSize + 1),
				createEbpfEvent(telemetryTestBufferSize + bucketLength + 1),
				createEbpfEvent(telemetryTestBufferSize + 2*bucketLength + 1),
				createEbpfEvent(telemetryTestBufferSize + 3*bucketLength + 1),
				createEbpfEvent(telemetryTestBufferSize + 4*bucketLength + 1),
				createEbpfEvent(telemetryTestBufferSize + 5*bucketLength + 1),
				createEbpfEvent(telemetryTestBufferSize + 6*bucketLength + 1),
			},
			maxBufferSize: telemetryTestBufferSize,

			expectedTelemetry: telemetryResults{
				queryLength:               [bucketLength]int64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
				failedOperationExtraction: 10,
				failedTableNameExtraction: 10,
			},
		},
		{
			name: "validating max buffer size which creates negative first bucket lower boundary",
			tx: []*ebpf.EbpfEvent{
				createEbpfEvent(ebpf.BufferSize - 2*bucketLength),
				createEbpfEvent(ebpf.BufferSize - bucketLength),
				createEbpfEvent(ebpf.BufferSize),
				createEbpfEvent(ebpf.BufferSize + 1),
				createEbpfEvent(ebpf.BufferSize + bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 2*bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 3*bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 4*bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 5*bucketLength + 1),
				createEbpfEvent(ebpf.BufferSize + 6*bucketLength + 1),
			},
			maxBufferSize: 1,

			expectedTelemetry: telemetryResults{
				queryLength:               [bucketLength]int64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
				failedOperationExtraction: 10,
				failedTableNameExtraction: 10,
			},
		},
		{
			name:  "failed operation extraction",
			tx:    []*ebpf.EbpfEvent{{}},
			query: "CREA TABLE dummy",
			expectedTelemetry: telemetryResults{
				failedOperationExtraction: 1,
				queryLength:               [bucketLength]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			},
		},
		{
			name:  "failed table name extraction",
			tx:    []*ebpf.EbpfEvent{{}},
			query: "CREATE TABLE",
			expectedTelemetry: telemetryResults{
				failedTableNameExtraction: 1,
				queryLength:               [bucketLength]int64{1, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			},
		},
		{
			name:  "failed table name and operation extraction",
			tx:    []*ebpf.EbpfEvent{{}},
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

			cfg := config.New()
			if tt.maxBufferSize > 0 {
				cfg.MaxPostgresTelemetryBuffer = tt.maxBufferSize
			}
			tel := NewTelemetry(cfg)
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

func createEbpfEvent(querySize int) *ebpf.EbpfEvent {
	return &ebpf.EbpfEvent{
		Tx: ebpf.EbpfTx{
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
