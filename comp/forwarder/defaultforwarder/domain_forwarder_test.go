// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package defaultforwarder

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/internal/retry"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestNewDomainForwarder(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 120*time.Second)

	assert.NotNil(t, forwarder)
	assert.Equal(t, 1, forwarder.numberOfWorkers)
	assert.Equal(t, 120*time.Second, forwarder.connectionResetInterval)
	assert.Equal(t, Stopped, forwarder.State())
	assert.Nil(t, forwarder.highPrio)
	assert.Nil(t, forwarder.lowPrio)
	assert.Nil(t, forwarder.requeuedTransaction)
	assert.Nil(t, forwarder.stopRetry)
	assert.Nil(t, forwarder.stopConnectionReset)
	assert.Len(t, forwarder.workers, 0)
	requireLenForwarderRetryQueue(t, forwarder, 0)
	assert.NotNil(t, forwarder.blockedList, 0)
}

func TestDomainForwarderStart(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	err := forwarder.Start()

	assert.Nil(t, err)
	requireLenForwarderRetryQueue(t, forwarder, 0)
	require.Len(t, forwarder.workers, 1)
	assert.Equal(t, Started, forwarder.State())
	assert.NotNil(t, forwarder.highPrio)
	assert.NotNil(t, forwarder.lowPrio)
	assert.NotNil(t, forwarder.requeuedTransaction)
	assert.NotNil(t, forwarder.stopRetry)
	assert.NotNil(t, forwarder.stopConnectionReset)

	assert.NotNil(t, forwarder.Start())

	forwarder.Stop(false)
}

func TestDomainForwarderInit(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	forwarder.init()
	assert.Len(t, forwarder.workers, 0)
	requireLenForwarderRetryQueue(t, forwarder, 0)
}

func TestDomainForwarderStop(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	forwarder.Stop(false) // this should be a noop
	forwarder.Start()
	assert.Equal(t, Started, forwarder.State())
	forwarder.Stop(false)
	assert.Len(t, forwarder.workers, 0)
	requireLenForwarderRetryQueue(t, forwarder, 0)
	assert.Equal(t, Stopped, forwarder.State())
}

func TestDomainForwarderStop_WithConnectionReset(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 120*time.Second)
	forwarder.Stop(false) // this should be a noop
	forwarder.Start()
	assert.Equal(t, Started, forwarder.State())
	forwarder.Stop(false)
	assert.Len(t, forwarder.workers, 0)
	requireLenForwarderRetryQueue(t, forwarder, 0)
	assert.Equal(t, Stopped, forwarder.State())
}

func TestDomainForwarderSendHTTPTransactions(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	tr := newTestTransactionDomainForwarder()

	// fw is stopped, we should get an error
	forwarder.sendHTTPTransactions(tr)

	defer forwarder.Stop(false)
	forwarder.Start()
	// Stopping the worker for the TestRequeueTransaction
	forwarder.workers[0].Stop(false)

	forwarder.sendHTTPTransactions(tr)
	transactionToProcess := <-forwarder.highPrio
	assert.Equal(t, tr, transactionToProcess)

	// Reset `forwarder.workers` otherwise `defer forwarder.Stop(false)` will timeout.
	forwarder.workers = nil
}

func TestRequeueTransaction(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	tr := transaction.NewHTTPTransaction()
	requireLenForwarderRetryQueue(t, forwarder, 0)
	forwarder.requeueTransaction(tr)
	requireLenForwarderRetryQueue(t, forwarder, 1)
}

func TestRetryTransactions(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	forwarder.init()

	// Default value should be 0
	assert.Equal(t, int64(0), transaction.TransactionsDropped.Value())

	payload := transaction.NewBytesPayloadWithoutMetaData([]byte{1})
	t1 := transaction.NewHTTPTransaction()
	t1.Domain = "domain/"
	t1.Endpoint.Route = "test1"
	t1.Payload = payload
	t2 := transaction.NewHTTPTransaction()
	t2.Domain = "domain/"
	t2.Endpoint.Route = "test2"
	t2.Payload = payload

	// Create blocks
	forwarder.blockedList.recover(t1.GetTarget())
	forwarder.blockedList.recover(t2.GetTarget())

	forwarder.blockedList.errorPerEndpoint[t1.GetTarget()].until = time.Now().Add(-1 * time.Hour)
	forwarder.blockedList.errorPerEndpoint[t2.GetTarget()].until = time.Now().Add(1 * time.Hour)

	forwarder.requeueTransaction(t2)
	forwarder.requeueTransaction(t2) // this second one should be dropped
	forwarder.requeueTransaction(t1) // the queue should be sorted
	forwarder.retryTransactions(time.Now())
	requireLenForwarderRetryQueue(t, forwarder, 1)
	assert.Len(t, forwarder.lowPrio, 1)
	assert.Equal(t, int64(1), transaction.TransactionsDropped.Value())
}

func TestForwarderRetry(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	forwarder.Start()
	defer forwarder.Stop(false)

	forwarder.blockedList.close("blocked")
	forwarder.blockedList.errorPerEndpoint["blocked"].until = time.Now().Add(1 * time.Hour)

	ready := newTestTransactionDomainForwarder()
	notReady := newTestTransactionDomainForwarder()

	forwarder.requeueTransaction(ready)
	forwarder.requeueTransaction(notReady)
	requireLenForwarderRetryQueue(t, forwarder, 2)

	ready.On("Process", forwarder.workers[0].Client).Return(nil).Times(1)
	ready.On("GetTarget").Return("").Times(2)
	ready.On("GetCreatedAt").Return(time.Now()).Times(1)
	notReady.On("GetCreatedAt").Return(time.Now()).Times(1)
	notReady.On("GetTarget").Return("blocked").Times(1)

	forwarder.retryTransactions(time.Now())
	<-ready.processed

	ready.AssertExpectations(t)
	notReady.AssertExpectations(t)
	notReady.AssertNumberOfCalls(t, "Process", 0)
	notReady.AssertNumberOfCalls(t, "GetTarget", 1)
	trs, err := forwarder.retryQueue.ExtractTransactions()
	require.NoError(t, err)
	require.Len(t, trs, 1)
	assert.Equal(t, trs[0], notReady)
}

func TestForwarderRetryLifo(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	forwarder.init()

	transaction1 := newTestTransactionDomainForwarder()
	transaction2 := newTestTransactionDomainForwarder()

	forwarder.requeueTransaction(transaction1)
	forwarder.requeueTransaction(transaction2)

	transaction1.On("GetCreatedAt").Return(time.Now()).Times(1)
	transaction1.On("GetTarget").Return("").Times(1)

	transaction2.On("GetCreatedAt").Return(time.Now().Add(1 * time.Minute)).Times(1)
	transaction2.On("GetTarget").Return("").Times(1)

	forwarder.retryTransactions(time.Now())

	firstOut := <-forwarder.lowPrio
	assert.Equal(t, firstOut, transaction2)

	secondOut := <-forwarder.lowPrio
	assert.Equal(t, secondOut, transaction1)

	transaction1.AssertExpectations(t)
	transaction2.AssertExpectations(t)
	requireLenForwarderRetryQueue(t, forwarder, 0)
}

func TestForwarderRetryLimitQueue(t *testing.T) {
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	forwarder.init()
	forwarder.blockedList.close("blocked")
	forwarder.blockedList.errorPerEndpoint["blocked"].until = time.Now().Add(1 * time.Minute)

	var transactions []*testTransaction
	for _, v := range []time.Time{time.Now(), time.Now().Add(1 * time.Minute), time.Now().Add(1 * time.Minute)} {
		transaction := newTestTransactionDomainForwarder()

		forwarder.requeueTransaction(transaction)
		transaction.On("GetCreatedAt").Return(v).Maybe()
		transaction.On("GetTarget").Return("blocked").Maybe()
		transactions = append(transactions, transaction)
	}

	forwarder.retryTransactions(time.Now())
	for _, tr := range transactions {
		tr.AssertExpectations(t)
	}

	require.Len(t, forwarder.highPrio, 0)
	require.Len(t, forwarder.lowPrio, 0)
	trs, err := forwarder.retryQueue.ExtractTransactions()
	require.NoError(t, err)
	require.Len(t, trs, 2)

	// assert that the oldest transaction was dropped
	assert.Equal(t, transactions[2], trs[0])
	assert.Equal(t, transactions[0], trs[1])
}

func TestDomainForwarderRetryQueueAllPayloadsMaxSize(t *testing.T) {
	oldFlushInterval := flushInterval
	defer func() { flushInterval = oldFlushInterval }()
	flushInterval = 1 * time.Minute

	telemetry := retry.NewTransactionRetryQueueTelemetry("domain")
	transactionRetryQueue := retry.NewTransactionRetryQueue(
		transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: true},
		nil,
		1+2,
		0,
		telemetry,
		retry.NewPointCountTelemetryMock())
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarder(mockConfig, "test", transactionRetryQueue, 0, 10, transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: true}, retry.NewPointCountTelemetry("domain", nil))
	forwarder.blockedList.close("blocked")
	forwarder.blockedList.errorPerEndpoint["blocked"].until = time.Now().Add(1 * time.Minute)

	defer forwarder.Stop(true)
	forwarder.Start()

	for _, payloadSize := range []int{4, 3, 2, 1} {
		tr := newTestTransaction()
		tr.On("GetPayloadSize").Return(payloadSize)
		tr.On("GetTarget").Return("blocked")
		tr.On("GetCreatedAt").Return(time.Now().Add(time.Duration(-payloadSize) * time.Second))
		transactionRetryQueue.Add(tr)
	}

	forwarder.retryTransactions(time.Now())

	trs, err := transactionRetryQueue.ExtractTransactions()
	require.NoError(t, err)
	require.Len(t, trs, 2)
	require.Equal(t, 1, trs[0].GetPayloadSize())
	require.Equal(t, 2, trs[1].GetPayloadSize())
}

func TestDomainForwarderInitConfigs(t *testing.T) {
	// Test default values
	mockConfig := pkgconfig.Mock(t)
	forwarder := newDomainForwarderForTest(mockConfig, 0)
	forwarder.init()
	assert.Equal(t, 100, cap(forwarder.highPrio))
	assert.Equal(t, 100, cap(forwarder.lowPrio))
	assert.Equal(t, 100, cap(forwarder.requeuedTransaction))

	// Test custom values
	datadogYaml := `
forwarder_high_prio_buffer_size: 1100
forwarder_low_prio_buffer_size: 1200
forwarder_requeue_buffer_size: 1300
`
	pkgconfig.Datadog.SetConfigType("yaml")
	err := pkgconfig.Datadog.ReadConfig(bytes.NewBuffer([]byte(datadogYaml)))
	assert.NoError(t, err)

	forwarder = newDomainForwarderForTest(mockConfig, 0)
	forwarder.init()
	assert.Equal(t, 1100, cap(forwarder.highPrio))
	assert.Equal(t, 1200, cap(forwarder.lowPrio))
	assert.Equal(t, 1300, cap(forwarder.requeuedTransaction))
}

func newDomainForwarderForTest(config config.Component, connectionResetInterval time.Duration) *domainForwarder {
	sorter := transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: true}
	telemetry := retry.NewTransactionRetryQueueTelemetry("domain")
	transactionRetryQueue := retry.NewTransactionRetryQueue(
		transaction.SortByCreatedTimeAndPriority{HighPriorityFirst: true},
		nil,
		2,
		0,
		telemetry,
		retry.NewPointCountTelemetryMock())

	return newDomainForwarder(config, "test", transactionRetryQueue, 1, connectionResetInterval, sorter, retry.NewPointCountTelemetry("domain", nil))
}

func requireLenForwarderRetryQueue(t *testing.T, forwarder *domainForwarder, expectedValue int) {
	require.Equal(t, expectedValue, forwarder.retryQueue.GetTransactionCount())
}

func newTestTransactionDomainForwarder() *testTransaction {
	tr := newTestTransaction()
	tr.On("GetPayloadSize").Return(1)
	return tr
}
