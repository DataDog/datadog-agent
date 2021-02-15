// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/go-multierror"
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
	telemetry                  transactionContainerTelemetry
	mutex                      sync.RWMutex
}

func getStorageMaxSize(storagePath string) (int64, error) {
	disk := filesystem.NewDisk()
	usage, err := disk.GetUsage(storagePath)
	if err != nil {
		return 0, err
	}
	storageMaxSize := config.Datadog.GetInt64("forwarder_storage_max_size_in_bytes")
	diskRatio := config.Datadog.GetFloat64("forwarder_storage_max_disk_ratio")
	minDiskUsage := config.Datadog.GetInt64("forwarder_storage_min_size_in_bytes")
	return getStorageMaxSizeFromDiskUsage(usage, storageMaxSize, diskRatio, minDiskUsage)
}

func getStorageMaxSizeFromDiskUsage(
	usage *filesystem.DiskUsage,
	storageMaxSize int64,
	diskRatio float64,
	minDiskUsage int64) (int64, error) {
	maxDiskUsage := float64(usage.Total) * diskRatio
	availableDiskUsage := int64(maxDiskUsage) - int64(usage.Used)
	if storageMaxSize > availableDiskUsage {
		log.Warnf("Cannot honor the value defined for `forwarder_storage_max_size_in_bytes=%v`. "+
			"The value exceeds the maximum disk space usage defined by `forwarder_storage_max_disk_ratio`. "+
			"`forwarder_storage_max_size_in_bytes=%v` is used instead. ", storageMaxSize, availableDiskUsage)
		storageMaxSize = availableDiskUsage
	}

	if storageMaxSize < minDiskUsage {
		return 0, fmt.Errorf("`forwarder_storage_max_size_in_bytes=%v` value is too low and must be at least %v", storageMaxSize, minDiskUsage)
	}
	return storageMaxSize, nil
}

func buildTransactionContainer(
	maxMemSizeInBytes int,
	flushToStorageRatio float64,
	optionalDomainFolderPath string,
	storageMaxSize int64,
	dropPrioritySorter transactionPrioritySorter,
	domain string,
	apiKeys []string) *transactionContainer {
	var storage transactionStorage
	var err error

	if optionalDomainFolderPath != "" && storageMaxSize > 0 {
		serializer := NewTransactionsSerializer(domain, apiKeys)
		storage, err = newTransactionsFileStorage(serializer, optionalDomainFolderPath, storageMaxSize, transactionsFileStorageTelemetry{})

		// If the storage on disk cannot be used, log the error and continue.
		// Returning `nil, err` would mean not using `TransactionContainer` and so not using `forwarder_retry_queue_payloads_max_size` config.
		if err != nil {
			log.Errorf("Error when creating the file storage: %v", err)
		}
	}

	return newTransactionContainer(dropPrioritySorter, storage, maxMemSizeInBytes, flushToStorageRatio, transactionContainerTelemetry{})
}

func newTransactionContainer(
	dropPrioritySorter transactionPrioritySorter,
	optionalTransactionStorage transactionStorage,
	maxMemSizeInBytes int,
	flushToStorageRatio float64,
	telemetry transactionContainerTelemetry) *transactionContainer {
	return &transactionContainer{
		maxMemSizeInBytes:          maxMemSizeInBytes,
		flushToStorageRatio:        flushToStorageRatio,
		dropPrioritySorter:         dropPrioritySorter,
		optionalTransactionStorage: optionalTransactionStorage,
		telemetry:                  telemetry,
	}
}

// add adds a new transaction and flush transactions to disk if the memory limit is exceeded.
// The amount of transactions flushed to disk is control by
// `flushToStorageRatio` which is the ratio of the transactions to be flushed.
// Consider the following payload sizes 10, 20, 30, 40, 15 with `maxMemSizeInBytes=100` and
// `flushToStorageRatio=0.6`
// When adding the last payload `15`, the buffer becomes full (10+20+30+40+15 > 100) and
// 100*0.6=60 bytes must be flushed on disk.
// The first 3 transactions are flushed to the disk as 10 + 20 + 30 >= 60
// If disk serialization failed or is not enabled, remove old transactions such as
// `currentMemSizeInBytes` <= `maxMemSizeInBytes`
func (tc *transactionContainer) add(t Transaction) (int, error) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()

	var diskErr error
	payloadSize := t.GetPayloadSize()
	if tc.optionalTransactionStorage != nil {
		payloadsGroupToFlush := tc.extractTransactionsForDisk(payloadSize)
		for _, payloads := range payloadsGroupToFlush {
			if err := tc.optionalTransactionStorage.Serialize(payloads); err != nil {
				diskErr = multierror.Append(diskErr, err)
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
		inMemTransactionDroppedCount = len(transactions)
		tc.telemetry.addTransactionsDroppedCount(inMemTransactionDroppedCount)
	}

	tc.transactions = append(tc.transactions, t)
	tc.currentMemSizeInBytes += payloadSize
	tc.telemetry.setCurrentMemSizeInBytes(tc.currentMemSizeInBytes)
	tc.telemetry.setTransactionsCount(len(tc.transactions))

	return inMemTransactionDroppedCount, diskErr
}

// extractTransactions extracts transactions from the container.
// If some transactions exist in memory extract them otherwise extract transactions
// from the disk.
// No transactions are in memory after calling this method.
func (tc *transactionContainer) extractTransactions() ([]Transaction, error) {
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
			tc.telemetry.incErrorsCount()
			return nil, err
		}
	}
	tc.currentMemSizeInBytes = 0
	tc.telemetry.setCurrentMemSizeInBytes(tc.currentMemSizeInBytes)
	tc.telemetry.setTransactionsCount(len(tc.transactions))
	return transactions, nil
}

// getCurrentMemSizeInBytes gets the current memory usage in bytes
func (tc *transactionContainer) getCurrentMemSizeInBytes() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return tc.currentMemSizeInBytes
}

// getTransactionCount gets the number of transactions in the container
func (tc *transactionContainer) getTransactionCount() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return len(tc.transactions)
}

// getMaxMemSizeInBytes gets the maximum memory usage for storing transactions
func (tc *transactionContainer) getMaxMemSizeInBytes() int {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()

	return tc.maxMemSizeInBytes
}

func (tc *transactionContainer) extractTransactionsForDisk(payloadSize int) [][]Transaction {
	sizeInBytesToFlush := int(float64(tc.maxMemSizeInBytes) * tc.flushToStorageRatio)
	var payloadsGroupToFlush [][]Transaction
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

func (tc *transactionContainer) extractTransactionsFromMemory(payloadSizeInBytesToExtract int) []Transaction {
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
