// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"fmt"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/ebpf"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	numberOfBuckets                         = 10
	bucketLength                            = 15
	numberOfBucketsSmallerThanMaxBufferSize = 3
)

type counterStateEnum int

const (
	tableAndOperation counterStateEnum = iota + 1
	operationNotFound
	tableNameNotFound
	tableAndOpNotFound
)

// extractionFailureCounter stores counter when goal was achieved and counter when target not found.
type extractionFailureCounter struct {
	// countTableAndOperationFound counts the number of successfully retrieved table name and operation.
	countTableAndOperationFound *libtelemetry.Counter
	// countOperationNotFound counts the number of unsuccessful fetches of the operation.
	countOperationNotFound *libtelemetry.Counter
	// countTableNameNotFound counts the number of unsuccessful fetches of the table name.
	countTableNameNotFound *libtelemetry.Counter
	// countTableAndOpNotFound counts the number of failed attempts to fetch both the table name and the operation.
	countTableAndOpNotFound *libtelemetry.Counter
}

// newExtractionFailureCounter creates and returns a new instance
func newExtractionFailureCounter(metricGroup *libtelemetry.MetricGroup, metricName string, tags ...string) *extractionFailureCounter {
	return &extractionFailureCounter{
		countTableAndOperationFound: metricGroup.NewCounter(metricName, append(tags, "state:table_and_op")...),
		countOperationNotFound:      metricGroup.NewCounter(metricName, append(tags, "state:no_operation")...),
		countTableNameNotFound:      metricGroup.NewCounter(metricName, append(tags, "state:no_table_name")...),
		countTableAndOpNotFound:     metricGroup.NewCounter(metricName, append(tags, "state:no_table_no_op")...),
	}
}

// inc increments the appropriate counter based on the provided state.
func (c *extractionFailureCounter) inc(state counterStateEnum) {
	switch state {
	case tableAndOperation:
		c.countTableAndOperationFound.Add(1)
	case operationNotFound:
		c.countOperationNotFound.Add(1)
	case tableNameNotFound:
		c.countTableNameNotFound.Add(1)
	case tableAndOpNotFound:
		c.countTableAndOpNotFound.Add(1)
	default:
		log.Errorf("unable to increment extractionFailureCounter due to undefined state: %v\n", state)
	}
}

// get returns the counter value based on the result.
func (c *extractionFailureCounter) get(state counterStateEnum) int64 {
	switch state {
	case tableAndOperation:
		return c.countTableAndOperationFound.Get()
	case operationNotFound:
		return c.countOperationNotFound.Get()
	case tableNameNotFound:
		return c.countTableNameNotFound.Get()
	case tableAndOpNotFound:
		return c.countTableAndOpNotFound.Get()
	default:
		return 0
	}
}

// Telemetry is a struct to hold the telemetry for the postgres protocol
type Telemetry struct {
	metricGroup *libtelemetry.MetricGroup

	// queryLengthBuckets holds the counters for the different buckets of by the query length quires
	queryLengthBuckets [numberOfBuckets]*extractionFailureCounter
	// failedTableNameExtraction holds the counter for the failed table name extraction
	failedTableNameExtraction *libtelemetry.Counter
	// failedOperationExtraction holds the counter for the failed operation extraction
	failedOperationExtraction *libtelemetry.Counter
	// firstBucketLowerBoundary is the lower boundary of the first bucket.
	// We inc 1 in order to include BufferSize as the upper boundary of the third bucket.
	// Then the first three buckets will include query lengths shorter or equal to BufferSize,
	// and the rest will include sizes equal to or above the buffer size.
	firstBucketLowerBoundary int
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
func createQueryLengthBuckets(metricGroup *libtelemetry.MetricGroup) [numberOfBuckets]*extractionFailureCounter {
	var buckets [numberOfBuckets]*extractionFailureCounter
	for i := 0; i < numberOfBuckets; i++ {
		buckets[i] = newExtractionFailureCounter(metricGroup, "query_length_bucket"+fmt.Sprint(i+1), libtelemetry.OptStatsd)
	}
	return buckets
}

// NewTelemetry creates a new Telemetry
func NewTelemetry(cfg *config.Config) *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.postgres")

	firstBucketLowerBoundary := cfg.MaxPostgresTelemetryBuffer - numberOfBucketsSmallerThanMaxBufferSize*bucketLength + 1
	if firstBucketLowerBoundary < 0 {
		log.Warnf("The first bucket lower boundary is negative: %d", firstBucketLowerBoundary)
		firstBucketLowerBoundary = ebpf.BufferSize - numberOfBucketsSmallerThanMaxBufferSize*bucketLength + 1
	}

	return &Telemetry{
		metricGroup:               metricGroup,
		queryLengthBuckets:        createQueryLengthBuckets(metricGroup),
		failedTableNameExtraction: metricGroup.NewCounter("failed_table_name_extraction", libtelemetry.OptStatsd),
		failedOperationExtraction: metricGroup.NewCounter("failed_operation_extraction", libtelemetry.OptStatsd),
		firstBucketLowerBoundary:  firstBucketLowerBoundary,
	}
}

// getBucketIndex returns the index of the bucket for the given query size
func (t *Telemetry) getBucketIndex(querySize int) int {
	bucketIndex := max(0, querySize-t.firstBucketLowerBoundary) / bucketLength
	return min(bucketIndex, numberOfBuckets-1)
}

// Count increments the telemetry counters based on the event data
func (t *Telemetry) Count(tx *ebpf.EbpfEvent, eventWrapper *EventWrapper) {
	querySize := int(tx.Tx.Original_query_size)

	state := tableAndOperation
	if eventWrapper.Operation() == UnknownOP {
		t.failedOperationExtraction.Add(1)
		state = operationNotFound
	}
	if eventWrapper.Parameters() == "UNKNOWN" {
		t.failedTableNameExtraction.Add(1)
		if state == operationNotFound {
			state = tableAndOpNotFound
		} else {
			state = tableNameNotFound
		}
	}
	bucketIndex := t.getBucketIndex(querySize)
	if bucketIndex >= 0 && bucketIndex < len(t.queryLengthBuckets) {
		t.queryLengthBuckets[bucketIndex].inc(state)
	}
}

// Log logs the postgres stats summary
func (t *Telemetry) Log() {
	if log.ShouldLog(seelog.DebugLvl) {
		log.Debugf("postgres stats summary: %s", t.metricGroup.Summary())
	}
}
