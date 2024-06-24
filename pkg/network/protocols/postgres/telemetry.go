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

const (
	numberOfBuckets        = 10
	bucketRange            = 15
	belowBufferBucketCount = 3
)

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

// createQueryLengthBuckets initializes the query length buckets
// Bucket 1: <= 33   query length
// Bucket 2: 34 - 48 query length
// Bucket 3: 49 - 63 query length
// Bucket 4: 64 - 78 query length
// Bucket 5: 79 - 93 query length
// Bucket 6: 94 - 108 query length
// Bucket 7: 109 - 123 query length
// Bucket 8: 124 - 138 query length
// Bucket 9: 139 - 153 query length
// Bucket 10: >= 154 query length
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
func getBucketIndex(querySize int) int {
	startSize := BufferSize - belowBufferBucketCount*bucketRange

	if querySize < startSize {
		return 0 // Bucket 1: queries smaller than the lower bound
	}

	bucketIndex := (querySize - startSize) / bucketRange
	if bucketIndex >= numberOfBuckets {
		return numberOfBuckets - 1 // Bucket 10: queries larger than the upper bound
	}
	return bucketIndex
}

// Count increments the telemetry counters based on the event data
func (t *Telemetry) Count(tx *EbpfEvent, eventWrapper *EventWrapper) {
	querySize := int(tx.Tx.Original_query_size)

	bucketIndex := getBucketIndex(querySize)
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
