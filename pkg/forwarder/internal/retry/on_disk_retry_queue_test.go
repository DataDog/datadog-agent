// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/stretchr/testify/assert"
)

const domainName = "domain"

func TestOnDiskRetryQueue(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	q := newTestOnDiskRetryQueue(a, path, 1000)
	err := q.Serialize(createHTTPTransactionCollectionTests("endpoint1", "endpoint2"))
	a.NoError(err)
	err = q.Serialize(createHTTPTransactionCollectionTests("endpoint3", "endpoint4"))
	a.NoError(err)
	a.Equal(2, q.getFilesCount())

	transactions, err := q.Deserialize()
	a.NoError(err)
	a.Equal([]string{"endpoint3", "endpoint4"}, getEndpointsFromTransactions(transactions))
	a.Greater(q.getCurrentSizeInBytes(), int64(0))

	transactions, err = q.Deserialize()
	a.NoError(err)
	a.Equal([]string{"endpoint1", "endpoint2"}, getEndpointsFromTransactions(transactions))
	a.Equal(0, q.getFilesCount())
	a.Equal(int64(0), q.getCurrentSizeInBytes())
}

func TestOnDiskRetryQueueMaxSize(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	maxSizeInBytes := int64(100)
	q := newTestOnDiskRetryQueue(a, path, maxSizeInBytes)

	i := 0
	err := q.Serialize(createHTTPTransactionCollectionTests(strconv.Itoa(i)))
	a.NoError(err)
	maxNumberOfFiles := int(maxSizeInBytes / q.getCurrentSizeInBytes())
	a.Greaterf(maxNumberOfFiles, 2, "Not enough files for this test, increase maxSizeInBytes")

	fileToDrop := 2
	for i++; i < maxNumberOfFiles+fileToDrop; i++ {
		err := q.Serialize(createHTTPTransactionCollectionTests(strconv.Itoa(i)))
		a.NoError(err)
	}
	a.LessOrEqual(q.getCurrentSizeInBytes(), maxSizeInBytes)
	a.Equal(maxNumberOfFiles, q.getFilesCount())

	for i--; i >= fileToDrop; i-- {
		transactions, err := q.Deserialize()
		a.NoError(err)
		a.Equal([]string{strconv.Itoa(i)}, getEndpointsFromTransactions(transactions))
	}

	a.Equal(0, q.getFilesCount())
}

func TestOnDiskRetryQueueReloadExistingRetryFiles(t *testing.T) {
	a := assert.New(t)
	path, clean := createTmpFolder(a)
	defer clean()

	retryQueue := newTestOnDiskRetryQueue(a, path, 1000)
	err := retryQueue.Serialize(createHTTPTransactionCollectionTests("endpoint1", "endpoint2"))
	a.NoError(err)

	newRetryQueue := newTestOnDiskRetryQueue(a, path, 1000)
	a.Equal(retryQueue.getCurrentSizeInBytes(), newRetryQueue.getCurrentSizeInBytes())
	a.Equal(retryQueue.getFilesCount(), newRetryQueue.getFilesCount())
	transactions, err := newRetryQueue.Deserialize()
	a.NoError(err)
	a.Equal([]string{"endpoint1", "endpoint2"}, getEndpointsFromTransactions(transactions))
}

func createHTTPTransactionCollectionTests(endpoints ...string) []transaction.Transaction {
	var transactions []transaction.Transaction

	for _, d := range endpoints {
		t := transaction.NewHTTPTransaction()
		t.Domain = domainName
		t.Endpoint.Name = d
		transactions = append(transactions, t)
	}
	return transactions
}

func createTmpFolder(a *assert.Assertions) (string, func()) {
	path, err := ioutil.TempDir("", "tests")
	a.NoError(err)
	return path, func() { _ = os.Remove(path) }
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
	telemetry := onDiskRetryQueueTelemetry{}
	disk := diskUsageRetrieverMock{
		diskUsage: &filesystem.DiskUsage{
			Available: 10000,
			Total:     10000,
		}}
	diskUsageLimit := newDiskUsageLimit("", disk, maxSizeInBytes, 1)
	storage, err := newOnDiskRetryQueue(NewHTTPTransactionsSerializer(domainName, nil), path, diskUsageLimit, telemetry)
	a.NoError(err)
	return storage
}
