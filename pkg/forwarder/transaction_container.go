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
	transactions          []Transaction
	currentMemSizeInBytes int
	maxMemSizeInBytes     int
	flushToStorageRatio   float32
	transactionStorage    transactionStorage
}

func newTransactionContainer(
	transactionStorage transactionStorage,
	maxMemSizeInBytes int,
	flushToStorageRatio float32) *transactionContainer {
	return &transactionContainer{
		maxMemSizeInBytes:   maxMemSizeInBytes,
		flushToStorageRatio: flushToStorageRatio,
		transactionStorage:  transactionStorage,
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
func (f *transactionContainer) Add(t Transaction) error {
	payloadSize := t.GetPayloadSize()
	if err := f.makeRoomFor(payloadSize); err != nil {
		return fmt.Errorf("Not enough space for the payload %v %v", t.GetTarget(), err)
	}

	f.transactions = append(f.transactions, t)
	f.currentMemSizeInBytes += payloadSize
	return nil
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
	} else {
		transactions, err = f.transactionStorage.Deserialize()
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

func (f *transactionContainer) makeRoomFor(payloadSize int) error {
	for f.currentMemSizeInBytes+payloadSize > f.maxMemSizeInBytes && len(f.transactions) > 0 {
		if err := f.flushToStorage(); err != nil {
			return err
		}
	}
	return nil
}

func (f *transactionContainer) flushToStorage() error {
	sizeInBytesFlushed := 0
	var payloadsToFlush []Transaction

	i := 0
	sizeInBytesToFlush := int(float32(f.maxMemSizeInBytes) * f.flushToStorageRatio)
	// Flush the N first transactions whose payload size sum is greater than `sizeInBytesToFlush`
	for ; i < len(f.transactions) && sizeInBytesFlushed < sizeInBytesToFlush; i++ {
		transaction := f.transactions[i]
		sizeInBytesFlushed += transaction.GetPayloadSize()
		payloadsToFlush = append(payloadsToFlush, transaction)
	}

	if len(payloadsToFlush) > 0 {
		err := f.transactionStorage.Serialize(payloadsToFlush)
		if err != nil {
			return err
		}
		f.transactions = f.transactions[i:]
		f.currentMemSizeInBytes -= sizeInBytesFlushed
	}

	return nil
}
