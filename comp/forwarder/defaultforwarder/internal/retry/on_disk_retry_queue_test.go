// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package retry

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

const domainName = "domain"

func TestOnDiskRetryQueue(t *testing.T) {
	a := assert.New(t)
	path := t.TempDir()

	pointDropped := fileStoragePointDroppedCountTelemetry.expvar.Value()
	q := newTestOnDiskRetryQueue(a, path, 1000)
	err := q.Store(createHTTPTransactionCollectionTests("endpoint1", "endpoint2"))
	a.NoError(err)
	err = q.Store(createHTTPTransactionCollectionTests("endpoint3", "endpoint4"))
	a.NoError(err)
	a.Equal(2, q.getFilesCount())
	a.Equal(pointDropped, fileStoragePointDroppedCountTelemetry.expvar.Value())

	transactions, err := q.ExtractLast()
	a.NoError(err)
	a.Equal([]string{"endpoint3", "endpoint4"}, getEndpointsFromTransactions(transactions))
	a.Greater(q.GetDiskSpaceUsed(), int64(0))

	transactions, err = q.ExtractLast()
	a.NoError(err)
	a.Equal([]string{"endpoint1", "endpoint2"}, getEndpointsFromTransactions(transactions))
	a.Equal(0, q.getFilesCount())
	a.Equal(int64(0), q.GetDiskSpaceUsed())
}

func TestOnDiskRetryQueueMaxSize(t *testing.T) {
	a := assert.New(t)
	path := t.TempDir()

	maxSizeInBytes := int64(100)
	pointDropped := fileStoragePointDroppedCountTelemetry.expvar.Value()
	q := newTestOnDiskRetryQueue(a, path, maxSizeInBytes)

	i := 0
	err := q.Store(createHTTPTransactionCollectionTests(strconv.Itoa(i)))
	a.NoError(err)
	maxNumberOfFiles := int(maxSizeInBytes / q.GetDiskSpaceUsed())
	a.Greaterf(maxNumberOfFiles, 2, "Not enough files for this test, increase maxSizeInBytes")

	fileToDrop := 2
	expectedPointDrop := int64(0)
	for i++; i < maxNumberOfFiles+fileToDrop; i++ {
		transactions := createHTTPTransactionCollectionTests(strconv.Itoa(i))
		err := q.Store(transactions)
		a.NoError(err)
		if i >= maxNumberOfFiles {
			for _, tr := range transactions {
				expectedPointDrop += int64(tr.GetPointCount())
			}
		}
	}
	a.LessOrEqual(q.GetDiskSpaceUsed(), maxSizeInBytes)
	a.Equal(maxNumberOfFiles, q.getFilesCount())

	a.Equal(pointDropped+expectedPointDrop, fileStoragePointDroppedCountTelemetry.expvar.Value())

	for i--; i >= fileToDrop; i-- {
		transactions, err := q.ExtractLast()
		a.NoError(err)
		a.Equal([]string{strconv.Itoa(i)}, getEndpointsFromTransactions(transactions))
	}

	a.Equal(0, q.getFilesCount())
}

func TestOnDiskRetryQueueReloadExistingRetryFiles(t *testing.T) {
	a := assert.New(t)
	path := t.TempDir()

	retryQueue := newTestOnDiskRetryQueue(a, path, 1000)
	err := retryQueue.Store(createHTTPTransactionCollectionTests("endpoint1", "endpoint2"))
	a.NoError(err)

	newRetryQueue := newTestOnDiskRetryQueue(a, path, 1000)
	a.Equal(retryQueue.GetDiskSpaceUsed(), newRetryQueue.GetDiskSpaceUsed())
	a.Equal(retryQueue.getFilesCount(), newRetryQueue.getFilesCount())
	transactions, err := newRetryQueue.ExtractLast()
	a.NoError(err)
	a.Equal([]string{"endpoint1", "endpoint2"}, getEndpointsFromTransactions(transactions))
}

func createHTTPTransactionCollectionTests(endpoints ...string) []transaction.Transaction {
	var transactions []transaction.Transaction

	for _, d := range endpoints {
		t := transaction.NewHTTPTransaction()
		t.Domain = domainName
		t.Endpoint.Name = d
		payload := make([]byte, 0)
		t.Payload = transaction.NewBytesPayload(payload, 1)
		transactions = append(transactions, t)
	}
	return transactions
}

func getEndpointsFromTransactions(transactions []transaction.Transaction) []string {
	var endpoints []string
	for _, t := range transactions {
		httpTransaction := t.(*transaction.HTTPTransaction)
		endpoints = append(endpoints, httpTransaction.Endpoint.Name)
	}
	return endpoints
}

func newTestOnDiskRetryQueue(a *assert.Assertions, path string, maxSizeInBytes int64) *onDiskRetryQueue {
	telemetry := newOnDiskRetryQueueTelemetry("domain")
	disk := diskUsageRetrieverMock{
		diskUsage: &filesystem.DiskUsage{
			Available: 10000,
			Total:     10000,
		}}
	diskUsageLimit := NewDiskUsageLimit("", disk, maxSizeInBytes, 1)
	storage, err := newOnDiskRetryQueue(NewHTTPTransactionsSerializer(resolver.NewSingleDomainResolver(domainName, nil)), path, diskUsageLimit, telemetry, NewPointCountTelemetryMock())
	a.NoError(err)
	return storage
}
