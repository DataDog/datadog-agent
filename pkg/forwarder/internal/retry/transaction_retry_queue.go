// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"fmt"
	"sync"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DiskTransactionSerializer is an interface to serialize / deserialize transactions
type DiskTransactionSerializer interface {
	Serialize([]transaction.Transaction) error
	Deserialize() ([]transaction.Transaction, error)
	GetDiskSpaceUsed() int64
}

// TransactionPrioritySorter is an interface to sort transactions.
type TransactionPrioritySorter interface {
	Sort([]transaction.Transaction)
}

// TransactionRetryQueue stores transactions in memory and flush them to disk when the memory
// limit is exceeded.
type TransactionRetryQueue struct {
	transactions          []transaction.Transaction
	currentMemSizeInBytes int
	maxMemSizeInBytes     int
	flushToStorageRatio   float64
	dropPrioritySorter    TransactionPrioritySorter
	optionalSerializer    DiskTransactionSerializer
	telemetry             TransactionRetryQueueTelemetry
	mutex                 sync.RWMutex
}

// BuildTransactionRetryQueue builds a new instance of TransactionRetryQueue
func BuildTransactionRetryQueue(
	maxMemSizeInBytes int,
	flushToStorageRatio float64,
	optionalDomainFolderPath string,
	optionalDiskUsageLimit *DiskUsageLimit,
	dropPrioritySorter TransactionPrioritySorter,
	resolver resolver.DomainResolver) *TransactionRetryQueue {
	var storage DiskTransactionSerializer
	var err error

	if optionalDomainFolderPath != "" && optionalDiskUsageLimit != nil {
		serializer := NewHTTPTransactionsSerializer(resolver)
		storage, err = newOnDiskRetryQueue(serializer, optionalDomainFolderPath, optionalDiskUsageLimit, newOnDiskRetryQueueTelemetry(resolver.GetBaseDomain()))

		// If the storage on disk cannot be used, log the error and continue.
		// Returning `nil, err` would mean not using `TransactionRetryQueue` and so not using `forwarder_retry_queue_payloads_max_size` config.
		if err != nil {
			log.Errorf("Error when creating the file storage: %v", err)
		}
	}

	return NewTransactionRetryQueue(
		dropPrioritySorter,
		storage,
		maxMemSizeInBytes,
		flushToStorageRatio,
		NewTransactionRetryQueueTelemetry(resolver.GetBaseDomain()))
}

// NewTransactionRetryQueue creates a new instance of NewTransactionRetryQueue
func NewTransactionRetryQueue(
	dropPrioritySorter TransactionPrioritySorter,
	optionalTransactionSerializer DiskTransactionSerializer,
	maxMemSizeInBytes int,
	flushToStorageRatio float64,
	telemetry TransactionRetryQueueTelemetry) *TransactionRetryQueue {
	return &TransactionRetryQueue{
		maxMemSizeInBytes:   maxMemSizeInBytes,
		flushToStorageRatio: flushToStorageRatio,
		dropPrioritySorter:  dropPrioritySorter,
		optionalSerializer:  optionalTransactionSerializer,
		telemetry:           telemetry,
	}
}

// Add adds a new transaction and flush transactions to disk if the memory limit is exceeded.
// The amount of transactions flushed to disk is control by
// `flushToStorageRatio` which is the ratio of the transactions to be flushed.
// Consider the following payload sizes 10, 20, 30, 40, 15 with `maxMemSizeInBytes=100` and
// `flushToStorageRatio=0.6`
// When adding the last payload `15`, the buffer becomes full (10+20+30+40+15 > 100) and
// 100*0.6=60 bytes must be flushed on disk.
// The first 3 transactions are flushed to the disk as 10 + 20 + 30 >= 60
// If disk serialization failed or is not enabled, remove old transactions such as
// `currentMemSizeInBytes` <= `maxMemSizeInBytes`
func (tc *TransactionRetryQueue) Add(t transaction.Transaction) (int, error) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	var diskErr error
	payloadSize := t.GetPayloadSize()
	if tc.optionalSerializer != nil {
		payloadsGroupToFlush := tc.extractTransactionsForDisk(payloadSize)
		for _, payloads := range payloadsGroupToFlush {
			if err := tc.optionalSerializer.Serialize(payloads); err != nil {
				diskErr = multierror.Append(diskErr, err)
				// Assuming all payloads failed during serialization
				for _, payload := range payloads {
					tc.telemetry.addPointDroppedCount(payload.GetPointCount())
				}
			}
		}
		if diskErr != nil {
			diskErr = fmt.Errorf("Cannot store transactions on disk: %v", diskErr)
			tc.telemetry.incErrorsCount()
		}
	}

	// If disk serialization failed or is not enabled, make sure `currentMemSizeInBytes` <= `maxMemSizeInBytes`
	payloadSizeInBytesToDrop := (tc.currentMemSizeInBytes + payloadSize) - tc.maxMemSizeInBytes
	inMemTransactionDroppedCount := 0
	if payloadSizeInBytesToDrop > 0 {
		transactions := tc.extractTransactionsFromMemory(payloadSizeInBytesToDrop)
		for _, tr := range transactions {
			tc.telemetry.addPointDroppedCount(tr.GetPointCount())
		}
		inMemTransactionDroppedCount = len(transactions)
		tc.telemetry.addTransactionsDroppedCount(inMemTransactionDroppedCount)
	}

	tc.transactions = append(tc.transactions, t)
	tc.currentMemSizeInBytes += payloadSize
	tc.telemetry.setCurrentMemSizeInBytes(tc.currentMemSizeInBytes)
	tc.telemetry.setTransactionsCount(len(tc.transactions))

	return inMemTransactionDroppedCount, diskErr
}

// ExtractTransactions extracts transactions from the container.
// If some transactions exist in memory extract them otherwise extract transactions
// from the disk.
// No transactions are in memory after calling this method.
func (tc *TransactionRetryQueue) ExtractTransactions() ([]transaction.Transaction, error) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	var transactions []transaction.Transaction
	var err error
	if len(tc.transactions) > 0 {
		transactions = tc.transactions
		tc.transactions = nil
	} else if tc.optionalSerializer != nil {
		transactions, err = tc.optionalSerializer.Deserialize()
		if err != nil {
			tc.telemetry.incErrorsCount()
			return nil, err
		}
	}
	tc.currentMemSizeInBytes = 0
	tc.telemetry.setCurrentMemSizeInBytes(tc.currentMemSizeInBytes)
	tc.telemetry.setTransactionsCount(len(tc.transactions))
	return transactions, nil
}

// GetCurrentMemSizeInBytes gets the current memory usage in bytes
func (tc *TransactionRetryQueue) getCurrentMemSizeInBytes() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return tc.currentMemSizeInBytes
}

// GetTransactionCount gets the number of transactions in the container
func (tc *TransactionRetryQueue) GetTransactionCount() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return len(tc.transactions)
}

// GetMaxMemSizeInBytes gets the maximum memory usage for storing transactions
func (tc *TransactionRetryQueue) GetMaxMemSizeInBytes() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return tc.maxMemSizeInBytes
}

// GetDiskSpaceUsed returns the current disk space used for storing transactions.
func (tc *TransactionRetryQueue) GetDiskSpaceUsed() int64 {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()
	if tc.optionalSerializer != nil {
		return tc.optionalSerializer.GetDiskSpaceUsed()
	}
	return 0
}

func (tc *TransactionRetryQueue) extractTransactionsForDisk(payloadSize int) [][]transaction.Transaction {
	sizeInBytesToFlush := int(float64(tc.maxMemSizeInBytes) * tc.flushToStorageRatio)
	var payloadsGroupToFlush [][]transaction.Transaction
	for tc.currentMemSizeInBytes+payloadSize > tc.maxMemSizeInBytes && len(tc.transactions) > 0 {
		// Flush the N first transactions whose payload size sum is greater than `sizeInBytesToFlush`
		transactions := tc.extractTransactionsFromMemory(sizeInBytesToFlush)

		if len(transactions) == 0 {
			// Happens when `sizeInBytesToFlush == 0`
			// Avoid infinite loop
			break
		}
		payloadsGroupToFlush = append(payloadsGroupToFlush, transactions)
	}

	return payloadsGroupToFlush
}

func (tc *TransactionRetryQueue) extractTransactionsFromMemory(payloadSizeInBytesToExtract int) []transaction.Transaction {
	i := 0
	sizeInBytesExtracted := 0
	var transactionsExtracted []transaction.Transaction

	tc.dropPrioritySorter.Sort(tc.transactions)
	for ; i < len(tc.transactions) && sizeInBytesExtracted < payloadSizeInBytesToExtract; i++ {
		transaction := tc.transactions[i]
		sizeInBytesExtracted += transaction.GetPayloadSize()
		transactionsExtracted = append(transactionsExtracted, transaction)
	}

	tc.transactions = tc.transactions[i:]
	tc.currentMemSizeInBytes -= sizeInBytesExtracted
	return transactionsExtracted
}
