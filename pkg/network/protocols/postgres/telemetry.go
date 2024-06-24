// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"fmt"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const numberOfBuckets = 4

// Telemetry is a struct to hold the telemetry for the postgres protocol
type Telemetry struct {
	metricGroup *libtelemetry.MetricGroup

	// exceededQueryLengthBuckets holds the counters for the different buckets of exceeding query length quires
	exceededQueryLengthBuckets [numberOfBuckets]*libtelemetry.Counter
	// failedTableNameExtraction holds the counter for the failed table name extraction
	failedTableNameExtraction *libtelemetry.Counter
	// failedOperationExtraction holds the counter for the failed operation extraction
	failedOperationExtraction *libtelemetry.Counter
}

// createQueryLengthBuckets initializes the query length buckets
func createQueryLengthBuckets(metricGroup *libtelemetry.MetricGroup) [numberOfBuckets]*libtelemetry.Counter {
	var buckets [numberOfBuckets]*libtelemetry.Counter
	for i := 0; i < numberOfBuckets; i++ {
		buckets[i] = metricGroup.NewCounter("exceeded_query_length_bucket"+fmt.Sprint(i), libtelemetry.OptStatsd)
	}
	return buckets
}

// NewTelemetry creates a new Telemetry
func NewTelemetry() *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.postgres")

	return &Telemetry{
		metricGroup:                metricGroup,
		exceededQueryLengthBuckets: createQueryLengthBuckets(metricGroup),
		failedTableNameExtraction:  metricGroup.NewCounter("failed_table_name_extraction", libtelemetry.OptStatsd),
		failedOperationExtraction:  metricGroup.NewCounter("failed_operation_extraction", libtelemetry.OptStatsd),
	}
}

// Count increments the telemetry counters based on the event data
func (t *Telemetry) Count(tx *EbpfEvent) {
	eventWrapper := NewEventWrapper(tx)
	querySize := tx.Tx.Original_query_size

	switch {
	case querySize > BufferSize*8:
		t.exceededQueryLengthBuckets[3].Add(1)
	case querySize > BufferSize*4:
		t.exceededQueryLengthBuckets[2].Add(1)
	case querySize > BufferSize*2:
		t.exceededQueryLengthBuckets[1].Add(1)
	case querySize > BufferSize:
		t.exceededQueryLengthBuckets[0].Add(1)
	}

	if eventWrapper.Operation() == UnknownOP {
		t.failedOperationExtraction.Add(1)
	}
	if eventWrapper.TableName() == "UNKNOWN" {
		t.failedTableNameExtraction.Add(1)
	}
}

// Log logs the postgres stats summary
func (t *Telemetry) Log() {
	log.Debugf("postgres stats summary: %s", t.metricGroup.Summary())
}
