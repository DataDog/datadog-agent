// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"fmt"
)

type transactionStorage interface {
	Serialize([]Transaction) error
	Deserialize() ([]Transaction, error)
}

// transactionContainer stores transactions in memory and flush them to disk when the memory
// limit is exceeded.
type transactionContainer struct {
	transactions               []Transaction
	currentMemSizeInBytes      int
	maxMemSizeInBytes          int
	flushToStorageRatio        float64
	optionalTransactionStorage transactionStorage
}

func newTransactionContainer(
	optionalTransactionStorage transactionStorage,
	maxMemSizeInBytes int,
	flushToStorageRatio float64) *transactionContainer {
	return &transactionContainer{
		maxMemSizeInBytes:          maxMemSizeInBytes,
		flushToStorageRatio:        flushToStorageRatio,
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
func (f *transactionContainer) Add(t Transaction) (int, error) {
	var diskErr error
	payloadSize := t.GetPayloadSize()
	if f.optionalTransactionStorage != nil {
		payloadsGroupToFlush := f.extractTransactionsForDisk(payloadSize)
		for _, payloads := range payloadsGroupToFlush {
			if err := f.optionalTransactionStorage.Serialize(payloads); err != nil {
				diskErr = fmt.Errorf("%v %v", diskErr, err)
			}
		}
		if diskErr != nil {
			diskErr = fmt.Errorf("Cannot store transactions on disk:%v", diskErr)
		}
	}

	// If disk serialization failed or is not enabled, make sure `currentMemSizeInBytes` <= `maxMemSizeInBytes`
	payloadSizeInBytesToDrop := (f.currentMemSizeInBytes + payloadSize) - f.maxMemSizeInBytes
	inMemTransactionDroppedCount := 0
	if payloadSizeInBytesToDrop > 0 {
		transactions := f.extractTransactions(payloadSizeInBytesToDrop)
		inMemTransactionDroppedCount = len(transactions)
	}

	f.transactions = append(f.transactions, t)
	f.currentMemSizeInBytes += payloadSize
	return inMemTransactionDroppedCount, diskErr
}

// ExtractTransactions extracts transactions from the container.
// If some transactions exist in memory extract them otherwise extract transactions
// from the disk.
// No transactions are in memory after calling this method.
func (f *transactionContainer) ExtractTransactions() ([]Transaction, error) {
	var transactions []Transaction
	var err error
	if len(f.transactions) > 0 {
		transactions = f.transactions
		f.transactions = nil
	} else if f.optionalTransactionStorage != nil {
		transactions, err = f.optionalTransactionStorage.Deserialize()
		if err != nil {
			return nil, err
		}
	}
	f.currentMemSizeInBytes = 0
	return transactions, nil
}

// GetCurrentMemSizeInBytes gets the current memory usage in bytes
func (f *transactionContainer) GetCurrentMemSizeInBytes() int {
	return f.currentMemSizeInBytes
}

// GetTransactionCount gets the number of transactions in the container
func (f *transactionContainer) GetTransactionCount() int {
	return len(f.transactions)
}

// GetMaxMemSizeInBytes gets the maximum memory usage for storing transactions
func (f *transactionContainer) GetMaxMemSizeInBytes() int {
	return f.maxMemSizeInBytes
}

func (f *transactionContainer) extractTransactionsForDisk(payloadSize int) [][]Transaction {
	sizeInBytesToFlush := int(float64(f.maxMemSizeInBytes) * f.flushToStorageRatio)
	var payloadsGroupToFlush [][]Transaction
	for f.currentMemSizeInBytes+payloadSize > f.maxMemSizeInBytes && len(f.transactions) > 0 {
		// Flush the N first transactions whose payload size sum is greater than `sizeInBytesToFlush`
		transactions := f.extractTransactions(sizeInBytesToFlush)

		payloadsGroupToFlush = append(payloadsGroupToFlush, transactions)
	}

	return payloadsGroupToFlush
}

func (f *transactionContainer) extractTransactions(payloadSizeInBytesToExtract int) []Transaction {
	i := 0
	sizeInBytesExtracted := 0
	var transactionsExtracted []Transaction

	for ; i < len(f.transactions) && sizeInBytesExtracted < payloadSizeInBytesToExtract; i++ {
		transaction := f.transactions[i]
		sizeInBytesExtracted += transaction.GetPayloadSize()
		transactionsExtracted = append(transactionsExtracted, transaction)
	}

	f.transactions = f.transactions[i:]
	f.currentMemSizeInBytes -= sizeInBytesExtracted
	return transactionsExtracted
}
