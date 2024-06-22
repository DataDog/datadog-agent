// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

type telemetryResults struct {
	exceededQueryLength       int64
	failedTableNameExtraction int64
	failedOperationExtraction int64
}

func TestTelemetry_Count(t *testing.T) {
	tests := []struct {
		name              string
		tx                *EbpfTx
		eventWrapper      *EventWrapper
		expectedTelemetry telemetryResults
	}{
		{
			name: "exceeded query length",
			tx: &EbpfTx{
				Original_query_size: 200,
			},
			eventWrapper: &EventWrapper{
				operation: SelectOP,
				tableName: "table",
			},
			expectedTelemetry: telemetryResults{
				exceededQueryLength: 1,
			},
		},
		{
			name: "failed operation extraction",
			tx: &EbpfTx{
				Original_query_size: 100,
			},
			eventWrapper: &EventWrapper{
				operation: UnknownOP,
				tableName: "table",
			},
			expectedTelemetry: telemetryResults{
				failedOperationExtraction: 1,
			},
		},
		{
			name: "failed table name extraction",
			tx: &EbpfTx{
				Original_query_size: 100,
			},
			eventWrapper: &EventWrapper{
				operation: SelectOP,
				tableName: "UNKNOWN",
			},
			expectedTelemetry: telemetryResults{
				failedTableNameExtraction: 1,
			},
		},
		{
			name: "failed table name and operation extraction",
			tx: &EbpfTx{
				Original_query_size: 100,
			},
			eventWrapper: &EventWrapper{
				operation: UnknownOP,
				tableName: "UNKNOWN",
			},
			expectedTelemetry: telemetryResults{
				failedTableNameExtraction: 1,
				failedOperationExtraction: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telemetry.Clear()
			tel := NewTelemetry()
			tel.Count(tt.tx, tt.eventWrapper)
			verifyTelemetry(t, tel, tt.expectedTelemetry)
		})
	}
}

func verifyTelemetry(t *testing.T, tel *Telemetry, expected telemetryResults) {
	assert.Equal(t, tel.exceededQueryLength.Get(), expected.exceededQueryLength, "exceededQueryLength count is incorrect")
	assert.Equal(t, tel.failedTableNameExtraction.Get(), expected.failedTableNameExtraction, "failedTableNameExtraction count is incorrect")
	assert.Equal(t, tel.failedOperationExtraction.Get(), expected.failedOperationExtraction, "failedOperationExtraction count is incorrect")
}
