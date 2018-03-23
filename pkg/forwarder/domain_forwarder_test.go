// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDomainForwarder(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)

	assert.NotNil(t, forwarder)
	assert.Equal(t, 1, forwarder.numberOfWorkers)
	assert.Equal(t, 10, forwarder.retryQueueLimit)
	assert.Equal(t, Stopped, forwarder.State())
	assert.Nil(t, forwarder.highPrio)
	assert.Nil(t, forwarder.lowPrio)
	assert.Nil(t, forwarder.requeuedTransaction)
	assert.Nil(t, forwarder.stopRetry)
	assert.Len(t, forwarder.workers, 0)
	assert.Len(t, forwarder.retryQueue, 0)
	assert.NotNil(t, forwarder.blockedList, 0)
}

func TestDomainForwarderStart(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)
	err := forwarder.Start()

	assert.Nil(t, err)
	require.Len(t, forwarder.retryQueue, 0)
	require.Len(t, forwarder.workers, 1)
	assert.Equal(t, Started, forwarder.State())
	assert.NotNil(t, forwarder.highPrio)
	assert.NotNil(t, forwarder.lowPrio)
	assert.NotNil(t, forwarder.requeuedTransaction)
	assert.NotNil(t, forwarder.stopRetry)

	assert.NotNil(t, forwarder.Start())

	forwarder.Stop()
}

func TestDomainForwarderInit(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)
	forwarder.init()
	assert.Len(t, forwarder.workers, 0)
	assert.Len(t, forwarder.retryQueue, 0)
}

func TestDomainForwarderStop(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)
	forwarder.Stop() // this should be a noop
	forwarder.Start()
	assert.Equal(t, Started, forwarder.State())
	forwarder.Stop()
	assert.Len(t, forwarder.workers, 0)
	assert.Len(t, forwarder.retryQueue, 0)
	assert.Equal(t, Stopped, forwarder.State())
}

func TestDomainForwarderSubmitIfStopped(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)

	require.NotNil(t, forwarder)
	assert.NotNil(t, forwarder.sendHTTPTransactions(nil))
}

func TestDomainForwarderSendHTTPTransactions(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)
	tr := newTestTransaction()

	// fw is stopped, we should get an error
	err := forwarder.sendHTTPTransactions(tr)
	assert.NotNil(t, err)

	forwarder.Start()
	// Stopping the worker for the TestRequeueTransaction
	forwarder.workers[0].Stop()

	err = forwarder.sendHTTPTransactions(tr)
	assert.Nil(t, err)
	transactionToProcess := <-forwarder.highPrio
	assert.Equal(t, tr, transactionToProcess)
}

func TestRequeueTransaction(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)
	tr := NewHTTPTransaction()
	assert.Len(t, forwarder.retryQueue, 0)
	forwarder.requeueTransaction(tr)
	assert.Len(t, forwarder.retryQueue, 1)
}

func TestRetryTransactions(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)
	forwarder.init()
	forwarder.retryQueueLimit = 1

	// Default value should be nil
	assert.Nil(t, transactionsExpvar.Get("Dropped"))

	t1 := NewHTTPTransaction()
	t1.Domain = "domain/"
	t1.Endpoint = "test1"
	t2 := NewHTTPTransaction()
	t2.Domain = "domain/"
	t2.Endpoint = "test2"

	// Create blocks
	forwarder.blockedList.recover(t1.GetTarget())
	forwarder.blockedList.recover(t2.GetTarget())

	forwarder.blockedList.errorPerEndpoint[t1.GetTarget()].until = time.Now().Add(-1 * time.Hour)
	forwarder.blockedList.errorPerEndpoint[t2.GetTarget()].until = time.Now().Add(1 * time.Hour)

	forwarder.requeueTransaction(t2)
	forwarder.requeueTransaction(t2) // this second one should be dropped
	forwarder.requeueTransaction(t1) // the queue should be sorted
	forwarder.retryTransactions(time.Now())
	assert.Len(t, forwarder.retryQueue, 1)
	assert.Len(t, forwarder.lowPrio, 1)
	require.NotNil(t, transactionsExpvar.Get("Dropped"))
	dropped, _ := strconv.ParseInt(transactionsExpvar.Get("Dropped").String(), 10, 64)
	assert.Equal(t, int64(1), dropped)
}

func TestForwarderRetry(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)
	forwarder.Start()
	defer forwarder.Stop()

	forwarder.blockedList.close("blocked")
	forwarder.blockedList.errorPerEndpoint["blocked"].until = time.Now().Add(1 * time.Hour)

	ready := newTestTransaction()
	notReady := newTestTransaction()

	forwarder.requeueTransaction(ready)
	forwarder.requeueTransaction(notReady)
	require.Len(t, forwarder.retryQueue, 2)

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
	require.Len(t, forwarder.retryQueue, 1)
	assert.Equal(t, forwarder.retryQueue[0], notReady)
}

func TestForwarderRetryLifo(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)
	forwarder.init()

	transaction1 := newTestTransaction()
	transaction2 := newTestTransaction()

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
	assert.Len(t, forwarder.retryQueue, 0)
}

func TestForwarderRetryLimitQueue(t *testing.T) {
	forwarder := newDomainForwarder("test", 1, 10)
	forwarder.init()

	forwarder.retryQueueLimit = 1
	forwarder.blockedList.close("blocked")
	forwarder.blockedList.errorPerEndpoint["blocked"].until = time.Now().Add(1 * time.Minute)

	transaction1 := newTestTransaction()
	transaction2 := newTestTransaction()

	forwarder.requeueTransaction(transaction1)
	forwarder.requeueTransaction(transaction2)

	transaction1.On("GetCreatedAt").Return(time.Now()).Times(1)
	transaction1.On("GetTarget").Return("blocked").Times(1)

	transaction2.On("GetCreatedAt").Return(time.Now().Add(1 * time.Minute)).Times(1)
	transaction2.On("GetTarget").Return("blocked").Times(1)

	forwarder.retryTransactions(time.Now())

	transaction1.AssertExpectations(t)
	transaction2.AssertExpectations(t)
	require.Len(t, forwarder.retryQueue, 1)
	require.Len(t, forwarder.highPrio, 0)
	require.Len(t, forwarder.lowPrio, 0)
	// assert that the oldest transaction was dropped
	assert.Equal(t, transaction2, forwarder.retryQueue[0])
}
