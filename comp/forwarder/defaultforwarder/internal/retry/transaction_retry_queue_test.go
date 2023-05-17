// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package retry

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
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

	container := NewTransactionRetryQueue(createDropPrioritySorter(), q, 100, 0.6, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

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

	container := NewTransactionRetryQueue(createDropPrioritySorter(), q, 50, 0.1, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

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

func TestTransactionRetryQueueNoTransactionStorage(t *testing.T) {
	a := assert.New(t)
	pointDropped := transactionContainerPointDroppedCountTelemetry.expvar.Value()
	container := NewTransactionRetryQueue(createDropPrioritySorter(), nil, 50, 0.1, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

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
	container := NewTransactionRetryQueue(createDropPrioritySorter(), q, maxMemSizeInBytes, 0.1, NewTransactionRetryQueueTelemetry("domain"), NewPointCountTelemetryMock())

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

func createTransactionWithPayloadSize(payloadSize int) *transaction.HTTPTransaction {
	tr := transaction.NewHTTPTransaction()
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

func createDropPrioritySorter() transaction.SortByCreatedTimeAndPriority {
	return transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: false}
}

func newOnDiskRetryQueueTest(t *testing.T, a *assert.Assertions) *onDiskRetryQueue {
	path := t.TempDir()
	disk := diskUsageRetrieverMock{
		diskUsage: &filesystem.DiskUsage{
			Available: 10000,
			Total:     10000,
		}}
	diskUsageLimit := NewDiskUsageLimit("", disk, 1000, 1)
	q, err := newOnDiskRetryQueue(
		NewHTTPTransactionsSerializer(resolver.NewSingleDomainResolver("", nil)),
		path,
		diskUsageLimit,
		newOnDiskRetryQueueTelemetry("domain"),
		NewPointCountTelemetryMock())
	a.NoError(err)
	return q
}
