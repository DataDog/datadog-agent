// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"fmt"
	"sync"
)

type retryTransactionsStorage interface {
	Serialize(payloadsToFlush []Transaction) error
	DeserializeLast() ([]Transaction, error)
	Stop() error
}

type retryTransactionsCollection struct {
	transactions             []Transaction
	currentPayloadsSize      int
	maxPayloadsSize          int
	retryTransactionsStorage retryTransactionsStorage
	mux                      sync.Mutex
}

func newRetryTransactionsCollection(
	maxPayloadsSize int,
	retryTransactionsFileStorage retryTransactionsStorage) *retryTransactionsCollection {
	return &retryTransactionsCollection{
		maxPayloadsSize:          maxPayloadsSize,
		retryTransactionsStorage: retryTransactionsFileStorage,
	}
}

// Add adds a new transaction
func (f *retryTransactionsCollection) Add(t Transaction) error {
	defer f.mux.Unlock()
	f.mux.Lock()
	payloadSize := t.GetPayloadSize()
	if err := f.makeRoomFor(payloadSize); err != nil {
		return fmt.Errorf("Not enough space for the payload %v %v", t.GetTarget(), err)
	}

	f.transactions = append(f.transactions, t)
	f.currentPayloadsSize += payloadSize
	return nil
}

// GetRetryTransactions gets the transactions for the next retry.
func (f *retryTransactionsCollection) GetRetryTransactions() ([]Transaction, error) {
	defer f.mux.Unlock()
	f.mux.Lock()

	var transactions []Transaction
	var err error
	if len(f.transactions) > 0 {
		transactions = f.transactions
		f.transactions = nil
		f.currentPayloadsSize = 0
	} else {
		transactions, err = f.retryTransactionsStorage.DeserializeLast()
		if err != nil {
			return nil, err
		}
	}
	return transactions, nil
}

// Stop stops and removes files.
func (f *retryTransactionsCollection) Stop() error {
	return f.retryTransactionsStorage.Stop()
}

func (f *retryTransactionsCollection) makeRoomFor(payloadSize int) error {
	for f.currentPayloadsSize+payloadSize > f.maxPayloadsSize && len(f.transactions) > 0 {
		if err := f.flushToStorage(); err != nil {
			return err
		}
	}
	return nil
}

func (f *retryTransactionsCollection) flushToStorage() error {
	payloadSizeToFlush := 0
	var payloadsToFlush []Transaction

	i := 0
	// Flush the N first transactions whose payload size sum is greater than maxPayloadsSize/2.
	for ; i < len(f.transactions) && payloadSizeToFlush < f.maxPayloadsSize/2; i++ {
		transaction := f.transactions[i]
		payloadSizeToFlush += transaction.GetPayloadSize()
		payloadsToFlush = append(payloadsToFlush, transaction)
	}

	if len(payloadsToFlush) > 0 {
		err := f.retryTransactionsStorage.Serialize(payloadsToFlush)
		if err != nil {
			return err
		}
		f.transactions = f.transactions[i:]
		f.currentPayloadsSize -= payloadSizeToFlush
	}

	return nil
}
