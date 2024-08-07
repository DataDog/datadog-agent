// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/ebpf"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	numberOfBuckets                         = 10
	bucketLength                            = 15
	numberOfBucketsSmallerThanMaxBufferSize = 3
)

// firstBucketLowerBoundary is the lower boundary of the first bucket.
// We add 1 in order to include BufferSize as the upper boundary of the third bucket.
// Then the first three buckets will include query lengths shorter or equal to BufferSize,
// and the rest will include sizes equal to or above the buffer size.
var firstBucketLowerBoundary = ebpf.BufferSize - numberOfBucketsSmallerThanMaxBufferSize*bucketLength + 1

// Telemetry is a struct to hold the telemetry for the postgres protocol
type Telemetry struct {
	metricGroup *libtelemetry.MetricGroup

	// queryLengthBuckets holds the counters for the different buckets of by the query length quires
	queryLengthBuckets [numberOfBuckets]*libtelemetry.Counter
	// failedTableNameExtraction holds the counter for the failed table name extraction
	failedTableNameExtraction *libtelemetry.Counter
	// failedOperationExtraction holds the counter for the failed operation extraction
	failedOperationExtraction *libtelemetry.Counter
}

// CountOptions holds the telemetry buffer size.
// The size is set by default to the ebpf.BufferSize
// but can be overridden by the max_postgres_telemetry_buffer configuration.
type CountOptions struct {
	TelemetryBufferSize int
}

// createQueryLengthBuckets initializes the query length buckets
// The buckets are defined relative to a `BufferSize` and a `bucketLength` as follows:
// Bucket 0: 0 to BufferSize - 2*bucketLength
// Bucket 1: BufferSize - 2*bucketLength + 1 to BufferSize - bucketLength
// Bucket 2: BufferSize - bucketLength + 1 to BufferSize
// Bucket 3: BufferSize + 1 to BufferSize + bucketLength
// Bucket 4: BufferSize + bucketLength + 1 to BufferSize + 2*bucketLength
// Bucket 5: BufferSize + 2*bucketLength + 1 to BufferSize + 3*bucketLength
// Bucket 6: BufferSize + 3*bucketLength + 1 to BufferSize + 4*bucketLength
// Bucket 7: BufferSize + 4*bucketLength + 1 to BufferSize + 5*bucketLength
// Bucket 8: BufferSize + 5*bucketLength + 1 to BufferSize + 6*bucketLength
// Bucket 9: BufferSize + 6*bucketLength + 1 to BufferSize + 7*bucketLength
func createQueryLengthBuckets(metricGroup *libtelemetry.MetricGroup) [numberOfBuckets]*libtelemetry.Counter {
	var buckets [numberOfBuckets]*libtelemetry.Counter
	for i := 0; i < numberOfBuckets; i++ {
		buckets[i] = metricGroup.NewCounter("query_length_bucket"+fmt.Sprint(i+1), libtelemetry.OptStatsd)
	}
	return buckets
}

// NewTelemetry creates a new Telemetry
func NewTelemetry() *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.postgres")

	return &Telemetry{
		metricGroup:               metricGroup,
		queryLengthBuckets:        createQueryLengthBuckets(metricGroup),
		failedTableNameExtraction: metricGroup.NewCounter("failed_table_name_extraction", libtelemetry.OptStatsd),
		failedOperationExtraction: metricGroup.NewCounter("failed_operation_extraction", libtelemetry.OptStatsd),
	}
}

// getBucketIndex returns the index of the bucket for the given query size
func getBucketIndex(querySize int, options ...CountOptions) int {
	if len(options) > 0 && options[0].TelemetryBufferSize > ebpf.BufferSize {
		firstBucketLowerBoundary = options[0].TelemetryBufferSize - numberOfBucketsSmallerThanMaxBufferSize*bucketLength + 1
	}
	bucketIndex := max(0, querySize-firstBucketLowerBoundary) / bucketLength
	return min(bucketIndex, numberOfBuckets-1)
}

// Count increments the telemetry counters based on the event data
func (t *Telemetry) Count(tx *ebpf.EbpfEvent, eventWrapper *EventWrapper, options ...CountOptions) {
	querySize := int(tx.Tx.Original_query_size)

	bucketIndex := getBucketIndex(querySize, options...)
	if bucketIndex >= 0 && bucketIndex < len(t.queryLengthBuckets) {
		t.queryLengthBuckets[bucketIndex].Add(1)
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
