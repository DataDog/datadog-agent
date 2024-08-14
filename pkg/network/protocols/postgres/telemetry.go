// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package postgres

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/postgres/ebpf"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
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

// tlsAwareCounter has a plain counter and a counter for TLS.
// It enables the use of a single metric that increments based on the encryption, avoiding the need for separate metrics for each use-case.
type tlsAwareCounter struct {
	counterPlain *libtelemetry.Counter
	counterTLS   *libtelemetry.Counter
}

// NewTLSAwareCounter creates and returns a new instance of the struct
func newTLSAwareCounter(metricGroup *libtelemetry.MetricGroup, metricName string, tags ...string) *tlsAwareCounter {
	return &tlsAwareCounter{
		counterPlain: metricGroup.NewCounter(metricName, append(tags, "encrypted:false")...),
		counterTLS:   metricGroup.NewCounter(metricName, append(tags, "encrypted:true")...),
	}
}

// Add adds the given delta to the counter based on the encryption.
func (c *tlsAwareCounter) add(delta int64, isTLS bool) {
	if isTLS {
		c.counterTLS.Add(delta)
		return
	}
	c.counterPlain.Add(delta)
}

// Get returns the counter value based on the encryption.
func (c *tlsAwareCounter) get(isTLS bool) int64 {
	if isTLS {
		return c.counterTLS.Get()
	}
	return c.counterPlain.Get()
}

// PGKernelTelemetry	provides empirical kernel statistics about the number of messages in each TCP packet
type PGKernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup
	// pgMessagesCountBuckets		count Postgres messages and divide the count into buckets
	pgMessagesCountBuckets [ebpf.KerMsgCountNumBuckets]*tlsAwareCounter
	// pgKernelMsgCountPlainPrev	save the last counters observed from the kernel, plain messages
	pgKernelMsgCountPlainPrev ebpf.PostgresKernelMsgCount
	// pgKernelMsgCountPrev			save the last counters observed from the kernel, TLS messages
	pgKernelMsgCountTlsPrev ebpf.PostgresKernelMsgCount
}

// newPGKernelTelemetry this is the Postgres message counter store.
func newPGKernelTelemetry() *PGKernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.postgres", libtelemetry.OptStatsd)
	pgKernelTel := &PGKernelTelemetry{
		metricGroup: metricGroup,
	}
	pgKernelTel.createMessagesCountBuckets(metricGroup)

	return pgKernelTel
}

func (t *PGKernelTelemetry) createMessagesCountBuckets(metricGroup *libtelemetry.MetricGroup) {
	for bucketIndex := range t.pgMessagesCountBuckets {
		t.pgMessagesCountBuckets[bucketIndex] = newTLSAwareCounter(metricGroup,
			"messages_count_bucket_"+(strconv.Itoa(bucketIndex+1)))
	}
}

// update the Postgres message counter store with new counters from the kernel.
// return if nothing to add, in this case also metric group summary ignores unmodified metrics.
func (t *PGKernelTelemetry) update(kernMsgCounts *ebpf.PostgresKernelMsgCount, isTLS bool) {
	if kernMsgCounts == nil {
		return
	}
	if isEmptyCounts(kernMsgCounts) {
		log.Debugf("postgres kernel telemetry empty.")
		return
	}
	delta := t.getDiff(kernMsgCounts, isTLS)
	if isEmptyCounts(delta) {
		log.Debugf("postgres kernel telemetry, no difference with previous counters.")
		return
	}
	for bucketIndex := range t.pgMessagesCountBuckets {
		t.pgMessagesCountBuckets[bucketIndex].add(int64(delta.Messages_count_buckets[bucketIndex]), isTLS)
	}
	if isTLS {
		t.pgKernelMsgCountTlsPrev = *kernMsgCounts
	} else {
		t.pgKernelMsgCountPlainPrev = *kernMsgCounts
	}
	s := t.metricGroup.Summary()
	log.Debugf("postgres kernel telemetry, isTLS=%t, summary: %s", isTLS, s)
}

func (t *PGKernelTelemetry) getDiff(kernMsgCounts *ebpf.PostgresKernelMsgCount, isTLS bool) *ebpf.PostgresKernelMsgCount {
	if isTLS {
		return subtractCounts(kernMsgCounts, &t.pgKernelMsgCountTlsPrev)
	}
	return subtractCounts(kernMsgCounts, &t.pgKernelMsgCountPlainPrev)
}

// subtractCounts create a new PostgresKernelMsgCount by subtracting the counts of 'b' from 'a'.
func subtractCounts(a *ebpf.PostgresKernelMsgCount, b *ebpf.PostgresKernelMsgCount) *ebpf.PostgresKernelMsgCount {
	res := &ebpf.PostgresKernelMsgCount{}
	res.Messages_count_buckets = subtractArrays(a.Messages_count_buckets, b.Messages_count_buckets)
	return res
}

// subtractArrays returns new array[] = a[] - b[]
// note that if b > a then the result is negative, although it is stored as an unsigned value giving a very large result.
func subtractArrays(a, b [ebpf.KerMsgCountNumBuckets]uint64) [ebpf.KerMsgCountNumBuckets]uint64 {
	var result [ebpf.KerMsgCountNumBuckets]uint64
	for i := 0; i < ebpf.KerMsgCountNumBuckets; i++ {
		result[i] = a[i] - b[i]
		if b[i] > a[i] {
			log.Debugf("postgres kernel telemetry, unexpected result(%u) = a(%u) - b(%u)", result[i], a[i], b[i])
		}
	}
	return result
}

func isEmptyCounts(p *ebpf.PostgresKernelMsgCount) bool {
	if !arrayIsEmpty(p.Messages_count_buckets) {
		return false
	}
	return true
}

func arrayIsEmpty(arr [ebpf.KerMsgCountNumBuckets]uint64) bool {
	for i := 0; i < ebpf.KerMsgCountNumBuckets; i++ {
		if arr[i] > 0 {
			return false
		}
	}
	return true
}
