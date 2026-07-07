// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package retry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// only used for testing
// GetCurrentMemSizeInBytes gets the current memory usage in bytes
func (tc *TransactionRetryQueue) getCurrentMemSizeInBytes() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return tc.currentMemSizeInBytes
}

func TestTransactionRetryQueueAdd(t *testing.T) {
	a := assert.New(t)
	pointDropped := transactionContainerPointDroppedCountTelemetry.expvar.Value()
	q := newOnDiskRetryQueueTest(t, a)

	container := NewTransactionRetryQueue(q, 100, 0.6, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

	// When adding the last element `15`, the buffer becomes full and the first 3
	// transactions are flushed to the disk as 10 + 20 + 30 >= 100 * 0.6
	for _, payloadSize := range []int{10, 20, 30, 40, 15} {
		_, err := container.Add(createTransactionWithPayloadSize(payloadSize))
		a.NoError(err)
	}
	a.Equal(40+15, container.getCurrentMemSizeInBytes())
	a.Equal(2, container.GetTransactionCount())
	a.Equal(pointDropped, transactionContainerPointDroppedCountTelemetry.expvar.Value())

	assertPayloadSizeFromExtractTransactions(a, container, []int{40, 15})

	_, err := container.Add(createTransactionWithPayloadSize(5))
	a.NoError(err)
	a.Equal(5, container.getCurrentMemSizeInBytes())
	a.Equal(1, container.GetTransactionCount())

	assertPayloadSizeFromExtractTransactions(a, container, []int{5})
	assertPayloadSizeFromExtractTransactions(a, container, []int{10, 20, 30})
	assertPayloadSizeFromExtractTransactions(a, container, nil)
}

func TestTransactionRetryQueueSeveralFlushToDisk(t *testing.T) {
	a := assert.New(t)
	q := newOnDiskRetryQueueTest(t, a)

	container := NewTransactionRetryQueue(q, 50, 0.1, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

	// Flush to disk when adding `40`
	for _, payloadSize := range []int{9, 10, 11, 40} {
		container.Add(createTransactionWithPayloadSize(payloadSize))
	}
	a.Equal(40, container.getCurrentMemSizeInBytes())
	a.Equal(3, q.getFilesCount())

	assertPayloadSizeFromExtractTransactions(a, container, []int{40})
	assertPayloadSizeFromExtractTransactions(a, container, []int{11})
	assertPayloadSizeFromExtractTransactions(a, container, []int{10})
	assertPayloadSizeFromExtractTransactions(a, container, []int{9})
	a.Equal(0, q.getFilesCount())
	a.Equal(int64(0), q.GetDiskSpaceUsed())
}

func TestTransactionRetryQueueFlushAllToDisk(t *testing.T) {
	a := assert.New(t)
	q := newOnDiskRetryQueueTest(t, a)

	container := NewTransactionRetryQueue(q, 50, 0.1, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

	// We're under the limit here, so should all be in memory
	for _, payloadSize := range []int{9, 10, 11} {
		container.Add(createTransactionWithPayloadSize(payloadSize))
	}
	a.Equal(30, container.getCurrentMemSizeInBytes())
	a.Equal(0, q.getFilesCount())

	// Flush to disk
	container.FlushToDisk()
	a.Equal(0, container.getCurrentMemSizeInBytes())
	a.Equal(1, q.getFilesCount())

	// Pull everything back out
	assertPayloadSizeFromExtractTransactions(a, container, []int{9, 10, 11})

	a.Equal(0, q.getFilesCount())
	a.Equal(int64(0), q.GetDiskSpaceUsed())
}

func TestTransactionRetryQueueNoTransactionStorage(t *testing.T) {
	a := assert.New(t)
	pointDropped := transactionContainerPointDroppedCountTelemetry.expvar.Value()
	container := NewTransactionRetryQueue(nil, 50, 0.1, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

	for _, payloadSize := range []int{9, 10, 11} {
		dropCount, err := container.Add(createTransactionWithPayloadSize(payloadSize))
		a.Equal(0, dropCount)
		a.NoError(err)
	}

	// Drop when adding `30`
	dropCount, err := container.Add(createTransactionWithPayloadSize(30))
	a.Equal(2, dropCount)
	a.NoError(err)
	a.Equal(pointDropped+2, transactionContainerPointDroppedCountTelemetry.expvar.Value())

	a.Equal(11+30, container.getCurrentMemSizeInBytes())

	assertPayloadSizeFromExtractTransactions(a, container, []int{11, 30})
}

func TestTransactionRetryQueueZeroMaxMemSizeInBytes(t *testing.T) {
	a := assert.New(t)
	q := newOnDiskRetryQueueTest(t, a)

	maxMemSizeInBytes := 0
	pointDropped := transactionContainerPointDroppedCountTelemetry.expvar.Value()
	container := NewTransactionRetryQueue(q, maxMemSizeInBytes, 0.1, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

	inMemTrDropped, err := container.Add(createTransactionWithPayloadSize(10))
	a.NoError(err)
	a.Equal(0, inMemTrDropped)
	a.Equal(pointDropped, transactionContainerPointDroppedCountTelemetry.expvar.Value())

	// `extractTransactionsForDisk` does not behave the same when there is a existing transaction.
	inMemTrDropped, err = container.Add(createTransactionWithPayloadSize(10))
	a.NoError(err)
	a.Equal(1, inMemTrDropped)
	a.Equal(pointDropped+1, transactionContainerPointDroppedCountTelemetry.expvar.Value())
}

// Verifies that when memory is tight, normal-priority transactions are dropped
// before high-priority ones regardless of insertion order.
//
// Setup: max=15, queue has high(5)+normal(5)=10. Adding new(10) needs 5 bytes freed.
// Extraction happens from the low-priority tail, so normal(5) is dropped, high(5) survives.
func TestTransactionRetryQueueDropsNormalPriorityBeforeHigh(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)
	container := NewTransactionRetryQueue(nil, 15, 0.1, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

	high := createTransactionWithPayloadSize(5)
	high.Priority = transaction.TransactionPriorityHigh
	_, err := container.Add(high)
	a.NoError(err)

	normal := createTransactionWithPayloadSize(5)
	normal.Priority = transaction.TransactionPriorityNormal
	_, err = container.Add(normal)
	a.NoError(err)

	// Adding a 10-byte item overflows by 5: (5+5+10)-15=5. The normal-priority transaction
	// (5 bytes) should be dropped; the high-priority one should survive.
	newTx := createTransactionWithPayloadSize(10)
	newTx.Priority = transaction.TransactionPriorityNormal
	dropCount, err := container.Add(newTx)
	a.NoError(err)
	a.Equal(1, dropCount)

	transactions, err := container.ExtractTransactions()
	a.NoError(err)
	r.Len(transactions, 2)
	priorities := []transaction.Priority{transactions[0].GetPriority(), transactions[1].GetPriority()}
	a.Contains(priorities, transaction.TransactionPriorityHigh)
}

// Verifies that among same-priority transactions, the oldest are dropped first
// when the memory limit is reached.
//
// Setup: max=25, add old(10)+middle(10)=20. Adding recent(10) overflows by 5,
// so one transaction must be dropped. The oldest (`old`) should be chosen.
func TestTransactionRetryQueueDropsOldestFirst(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)
	container := NewTransactionRetryQueue(nil, 25, 0.1, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

	old := createTransactionWithPayloadSize(10)
	old.Priority = transaction.TransactionPriorityNormal
	middle := createTransactionWithPayloadSize(10)
	middle.Priority = transaction.TransactionPriorityNormal
	recent := createTransactionWithPayloadSize(10)
	recent.Priority = transaction.TransactionPriorityNormal
	// Ensure deterministic creation-time ordering by explicit assignment.
	old.CreatedAt = old.CreatedAt.Add(-2 * time.Second)
	middle.CreatedAt = middle.CreatedAt.Add(-1 * time.Second)

	_, _ = container.Add(middle)
	_, _ = container.Add(old)

	// Adding `recent` overflows by 5: (20+10)-25=5. Should drop `old` (oldest, 10 bytes >= 5).
	dropCount, err := container.Add(recent)
	a.NoError(err)
	a.Equal(1, dropCount)

	transactions, err := container.ExtractTransactions()
	a.NoError(err)
	r.Len(transactions, 2)
	// middle and recent survive; old was dropped.
	survivingTimes := []time.Time{transactions[0].GetCreatedAt(), transactions[1].GetCreatedAt()}
	a.Contains(survivingTimes, middle.CreatedAt)
	a.Contains(survivingTimes, recent.CreatedAt)
	a.NotContains(survivingTimes, old.CreatedAt)
}

func createTransactionWithPayloadSize(payloadSize int) *transaction.HTTPTransaction {
	tr := transaction.NewHTTPTransaction()
	tr.CreatedAt = time.Unix(int64(payloadSize), 0)
	payload := make([]byte, payloadSize)
	tr.Payload = transaction.NewBytesPayload(payload, 1)
	return tr
}

func assertPayloadSizeFromExtractTransactions(
	a *assert.Assertions,
	container *TransactionRetryQueue,
	expectedPayloadSize []int) {

	transactions, err := container.ExtractTransactions()
	a.NoError(err)
	a.Equal(0, container.getCurrentMemSizeInBytes())

	var payloadSizes []int
	for _, t := range transactions {
		payloadSizes = append(payloadSizes, t.GetPayloadSize())
	}
	a.EqualValues(expectedPayloadSize, payloadSizes)
}

func newOnDiskRetryQueueTest(t *testing.T, a *assert.Assertions) *onDiskRetryQueue {
	path := t.TempDir()
	disk := diskUsageRetrieverMock{
		diskUsage: &filesystem.DiskUsage{
			Available: 10000,
			Total:     10000,
		}}
	diskUsageLimit := NewDiskUsageLimit("", disk, 1000, 1)
	log := logmock.New(t)
	r, err := resolver.NewSingleDomainResolver("", []utils.APIKeys{utils.NewAPIKeys("path", "api-key-1")})
	require.NoError(t, err)
	q, err := newOnDiskRetryQueue(
		log,
		NewHTTPTransactionsSerializer(log, r),
		path,
		diskUsageLimit,
		newOnDiskRetryQueueTelemetry("domain"),
		NewPointCountTelemetryMock())
	a.NoError(err)
	return q
}
