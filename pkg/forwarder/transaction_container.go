// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"fmt"
	"sync"
)

type transactionStorage interface {
	Serialize([]Transaction) error
	Deserialize() ([]Transaction, error)
}

type transactionPrioritySorter interface {
	Sort([]Transaction)
}

// transactionContainer stores transactions in memory and flush them to disk when the memory
// limit is exceeded.
type transactionContainer struct {
	transactions               []Transaction
	currentMemSizeInBytes      int
	maxMemSizeInBytes          int
	flushToStorageRatio        float64
	dropPrioritySorter         transactionPrioritySorter
	optionalTransactionStorage transactionStorage
	mutex                      sync.RWMutex
}

func newTransactionContainer(
	dropPrioritySorter transactionPrioritySorter,
	optionalTransactionStorage transactionStorage,
	maxMemSizeInBytes int,
	flushToStorageRatio float64) *transactionContainer {
	return &transactionContainer{
		maxMemSizeInBytes:          maxMemSizeInBytes,
		flushToStorageRatio:        flushToStorageRatio,
		dropPrioritySorter:         dropPrioritySorter,
		optionalTransactionStorage: optionalTransactionStorage,
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
func (tc *transactionContainer) Add(t Transaction) (int, error) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	var diskErr error
	payloadSize := t.GetPayloadSize()
	if tc.optionalTransactionStorage != nil {
		payloadsGroupToFlush := tc.extractTransactionsForDisk(payloadSize)
		for _, payloads := range payloadsGroupToFlush {
			if err := tc.optionalTransactionStorage.Serialize(payloads); err != nil {
				diskErr = fmt.Errorf("%v %v", diskErr, err)
			}
		}
		if diskErr != nil {
			diskErr = fmt.Errorf("Cannot store transactions on disk:%v", diskErr)
		}
	}

	// If disk serialization failed or is not enabled, make sure `currentMemSizeInBytes` <= `maxMemSizeInBytes`
	payloadSizeInBytesToDrop := (tc.currentMemSizeInBytes + payloadSize) - tc.maxMemSizeInBytes
	inMemTransactionDroppedCount := 0
	if payloadSizeInBytesToDrop > 0 {
		transactions := tc.extractTransactions(payloadSizeInBytesToDrop)
		inMemTransactionDroppedCount = len(transactions)
	}

	tc.transactions = append(tc.transactions, t)
	tc.currentMemSizeInBytes += payloadSize
	return inMemTransactionDroppedCount, diskErr
}

// ExtractTransactions extracts transactions from the container.
// If some transactions exist in memory extract them otherwise extract transactions
// from the disk.
// No transactions are in memory after calling this method.
func (tc *transactionContainer) ExtractTransactions() ([]Transaction, error) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	var transactions []Transaction
	var err error
	if len(tc.transactions) > 0 {
		transactions = tc.transactions
		tc.transactions = nil
	} else if tc.optionalTransactionStorage != nil {
		transactions, err = tc.optionalTransactionStorage.Deserialize()
		if err != nil {
			return nil, err
		}
	}
	tc.currentMemSizeInBytes = 0
	return transactions, nil
}

// GetCurrentMemSizeInBytes gets the current memory usage in bytes
func (tc *transactionContainer) GetCurrentMemSizeInBytes() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return tc.currentMemSizeInBytes
}

// GetTransactionCount gets the number of transactions in the container
func (tc *transactionContainer) GetTransactionCount() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return len(tc.transactions)
}

// GetMaxMemSizeInBytes gets the maximum memory usage for storing transactions
func (tc *transactionContainer) GetMaxMemSizeInBytes() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return tc.maxMemSizeInBytes
}

func (tc *transactionContainer) extractTransactionsForDisk(payloadSize int) [][]Transaction {
	sizeInBytesToFlush := int(float64(tc.maxMemSizeInBytes) * tc.flushToStorageRatio)
	var payloadsGroupToFlush [][]Transaction
	for tc.currentMemSizeInBytes+payloadSize > tc.maxMemSizeInBytes && len(tc.transactions) > 0 {
		// Flush the N first transactions whose payload size sum is greater than `sizeInBytesToFlush`
		transactions := tc.extractTransactions(sizeInBytesToFlush)

		payloadsGroupToFlush = append(payloadsGroupToFlush, transactions)
	}

	return payloadsGroupToFlush
}

func (tc *transactionContainer) extractTransactions(payloadSizeInBytesToExtract int) []Transaction {
	i := 0
	sizeInBytesExtracted := 0
	var transactionsExtracted []Transaction

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
