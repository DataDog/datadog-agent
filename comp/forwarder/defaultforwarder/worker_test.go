// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package defaultforwarder

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	tmock "github.com/stretchr/testify/mock"
	"go.uber.org/atomic"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"

	mock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNewWorker(t *testing.T) {
	highPrio := make(chan transaction.Transaction)
	lowPrio := make(chan transaction.Transaction)
	requeue := make(chan transaction.Transaction)

	mockConfig := mock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig))
	assert.NotNil(t, w)
	assert.Equal(t, w.Client.GetClient().Timeout, mockConfig.GetDuration("forwarder_timeout")*time.Second)
}

func TestNewNoSSLWorker(t *testing.T) {
	highPrio := make(chan transaction.Transaction)
	lowPrio := make(chan transaction.Transaction)
	requeue := make(chan transaction.Transaction)

	mockConfig := mock.New(t)
	mockConfig.SetInTest("skip_ssl_validation", true)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig))
	assert.True(t, w.Client.GetClient().Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
}

func TestWorkerStart(t *testing.T) {
	highPrio := make(chan transaction.Transaction)
	lowPrio := make(chan transaction.Transaction)
	requeue := make(chan transaction.Transaction, 1)
	sender := &PointSuccessfullySentMock{}
	mockConfig := mock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), sender, NewSharedConnection(log, false, 1, mockConfig))

	mock := newTestTransaction()
	mock.pointCount = 1
	mock.On("Process", w.Client.GetClient()).Return(nil).Times(1)
	mock.On("GetTarget").Return("").Times(1)
	mock2 := newTestTransaction()
	mock2.pointCount = 2
	mock2.On("Process", w.Client.GetClient()).Return(nil).Times(1)
	mock2.On("GetTarget").Return("").Times(1)

	w.Start()

	highPrio <- mock
	<-mock.processed

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)

	lowPrio <- mock2
	<-mock2.processed

	mock2.AssertExpectations(t)
	mock2.AssertNumberOfCalls(t, "Process", 1)

	// use Stop to wait for worker.process to update sender.count
	w.Stop(false)

	assert.Equal(t, int64(mock.pointCount+mock2.pointCount), sender.count.Load())
}

func TestWorkerRetry(t *testing.T) {
	highPrio := make(chan transaction.Transaction)
	lowPrio := make(chan transaction.Transaction)
	requeue := make(chan transaction.Transaction, 1)
	mockConfig := mock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig))

	mock := newTestTransaction()
	mock.On("Process", w.Client.GetClient()).Return(errors.New("some kind of error")).Times(1)
	mock.On("GetTarget").Return("error_url").Times(1)

	w.Start()
	highPrio <- mock
	retryTransaction := <-requeue
	w.Stop(false)
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)
	mock.AssertNumberOfCalls(t, "GetTarget", 1)
	assert.Equal(t, mock, retryTransaction)
	assert.True(t, w.blockedList.isBlockForSend("error_url", time.Now()))
}

func TestWorkerRetryBlockedTransaction(t *testing.T) {
	highPrio := make(chan transaction.Transaction)
	lowPrio := make(chan transaction.Transaction)
	requeue := make(chan transaction.Transaction, 1)
	mockConfig := mock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig))

	mock := newTestTransaction()
	mock.On("GetTarget").Return("error_url").Times(1)

	w.blockedList.close("error_url", time.Now())
	w.Start()
	highPrio <- mock
	retryTransaction := <-requeue
	w.Stop(false)
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 0)
	mock.AssertNumberOfCalls(t, "GetTarget", 1)
	assert.Equal(t, mock, retryTransaction)
	assert.True(t, w.blockedList.isBlockForSend("error_url", time.Now()))
}

func TestWorkerResetConnections(t *testing.T) {
	highPrio := make(chan transaction.Transaction)
	lowPrio := make(chan transaction.Transaction)
	requeue := make(chan transaction.Transaction, 1)
	mockConfig := mock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	connection := NewSharedConnection(log, false, 1, mockConfig)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, connection)

	mock := newTestTransaction()
	mock.On("Process", w.Client.GetClient()).Return(nil).Times(1)
	mock.On("GetTarget").Return("").Times(1)

	w.Start()

	highPrio <- mock
	<-mock.processed

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)

	httpClientBefore := w.Client.GetClient()
	connection.ResetClient()

	// tricky to test here that "Process" is called with a new http client
	mock2 := newTestTransactionWithoutClientAssert()
	mock2.On("Process").Return(nil).Times(1)
	mock2.On("GetTarget").Return("").Times(1)
	highPrio <- mock2
	<-mock2.processed
	mock2.AssertExpectations(t)
	mock2.AssertNumberOfCalls(t, "Process", 1)

	assert.NotSame(t, httpClientBefore, w.Client.GetClient())

	mock3 := newTestTransaction()
	mock3.On("Process", w.Client.GetClient()).Return(nil).Times(1)
	mock3.On("GetTarget").Return("").Times(1)
	highPrio <- mock3
	<-mock3.processed
	mock3.AssertExpectations(t)
	mock3.AssertNumberOfCalls(t, "Process", 1)

	w.Stop(false)
}

func TestWorkerCancelsInFlight(t *testing.T) {
	highPrio := make(chan transaction.Transaction, 1)
	lowPrio := make(chan transaction.Transaction, 1)
	requeue := make(chan transaction.Transaction, 1)

	// Wait to ensure the server has fully started
	stop := make(chan struct{}, 1)

	// Wait to ensure the server has finished stopping
	stopped := make(chan struct{}, 1)

	mockConfig := mock.New(t)

	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig))

	go func() {
		w.Start()
		<-stop
		w.Stop(false)
		stopped <- struct{}{}
	}()

	var processedwg sync.WaitGroup
	processedwg.Add(1)

	mockTransaction := newTestTransaction()
	mockTransaction.shouldBlock = true
	mockTransaction.
		On("Process", w.Client.GetClient()).
		Run(func(_args tmock.Arguments) {
			processedwg.Done()
		}).
		Return(errors.New("Cancelled")).Times(1)

	mockTransaction.On("GetTarget").Return("").Times(1)

	highPrio <- mockTransaction

	processedwg.Wait()
	stop <- struct{}{}

	// Wait for the server to fully stop
	<-stopped

	select {
	case requeued := <-requeue:
		assert.Equal(t, mockTransaction, requeued)
	default:
		assert.Fail(t, "Transaction not requeued")
	}
}

func TestWorkerCancelsWaitingTransactions(t *testing.T) {
	highPrio := make(chan transaction.Transaction, 10)
	lowPrio := make(chan transaction.Transaction, 10)
	requeue := make(chan transaction.Transaction, 10)

	// Wait to ensure the server has fully started
	stop := make(chan struct{}, 1)

	// Wait to ensure the server has finished stopping
	stopped := make(chan struct{}, 1)

	mockConfig := mock.New(t)

	log := logmock.New(t)

	// Configure the worker to have 3 maximum concurrent requests
	requests := 3

	mockConfig.SetInTest("forwarder_max_concurrent_requests", requests)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, requests, mockConfig))

	go func() {
		w.Start()
		<-stop
		w.Stop(true)
		stopped <- struct{}{}
	}()

	var processedwg sync.WaitGroup
	processedwg.Add(requests)

	// Create enough transactions that some will be waiting on the semaphore when we cancel.
	transactions := []*testTransaction{}
	for i := 0; i < 5; i++ {
		mockTransaction := newTestTransaction()

		// The first three transactions will block when sent to ensure they are currently holding
		// the semaphore when we want to stop the server. When we cancel the semaphore, these should
		// exit with an error.
		if i <= 3 {
			mockTransaction.shouldBlock = true
			mockTransaction.
				On("Process", w.Client.GetClient()).
				Run(func(_args tmock.Arguments) {
					processedwg.Done()
				}).
				Return(errors.New("Cancelled")).Times(1)
		} else {
			// The other transactions succeed.
			mockTransaction.On("Process", w.Client.GetClient()).Return(nil).Times(1)
		}

		// Each transaction has a different target to ensure the block list doesn't prevent
		// transactions being processed on too many errors for a target.
		mockTransaction.On("GetTarget").Return("Target" + strconv.Itoa(i))

		transactions = append(transactions, mockTransaction)
		highPrio <- mockTransaction
	}

	// Wait for three (the number of concurrent requests allowed) process calls to be made.
	processedwg.Wait()

	// Ensure the four possible transactions (three that have been processed, and one that doesn't get
	// past the semaphore) have been processed by the `Start` loop.
	for len(highPrio) > 1 {
		time.Sleep(time.Nanosecond)
	}

	stop <- struct{}{}

	// Wait for the server to fully stop
	<-stopped

	// The first three transactions get through the semaphore and are processed.
	transactions[0].AssertNumberOfCalls(t, "Process", 1)
	transactions[1].AssertNumberOfCalls(t, "Process", 1)
	transactions[2].AssertNumberOfCalls(t, "Process", 1)
	// The fourth transaction was waiting on the semaphore.
	// The semaphore was cancelled so it gets requeued straight away.
	transactions[3].AssertNumberOfCalls(t, "Process", 0)

	// The final transaction is resent due to passing `purgeHighPrio=true` to
	// the `Stop` function that will attempt to send anything waiting in the
	// high priority queue.
	transactions[4].AssertNumberOfCalls(t, "Process", 1)

	// 4 transactions get requeued, the first three that were cancelled inflight
	// and the fourth one that was stuck waiting on the MaxRequests semaphore.
	assert.Equal(t, 4, len(requeue))
}

func TestWorkerPurgeOnStop(t *testing.T) {
	highPrio := make(chan transaction.Transaction, 1)
	lowPrio := make(chan transaction.Transaction, 1)
	requeue := make(chan transaction.Transaction, 1)
	mockConfig := mock.New(t)
	log := logmock.New(t)

	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig))
	close(w.stopped)

	mockTransaction := newTestTransaction()
	mockTransaction.On("Process", w.Client.GetClient()).Return(nil).Times(1)
	mockTransaction.On("GetTarget").Return("").Times(1)
	highPrio <- mockTransaction

	mockRetryTransaction := newTestTransaction()
	mockRetryTransaction.On("Process", w.Client.GetClient()).Return(nil).Times(1)
	mockRetryTransaction.On("GetTarget").Return("").Times(1)
	lowPrio <- mockRetryTransaction

	// First test without purging
	w.Stop(false)
	mockTransaction.AssertNumberOfCalls(t, "Process", 0)
	mockRetryTransaction.AssertNumberOfCalls(t, "Process", 0)

	// Then with purging new transactions only
	w.Stop(true)
	mockTransaction.AssertExpectations(t)
	mockTransaction.AssertNumberOfCalls(t, "Process", 1)
	mockRetryTransaction.AssertNumberOfCalls(t, "Process", 0)
}

type PointSuccessfullySentMock struct {
	count atomic.Int64
}

func (m *PointSuccessfullySentMock) OnPointSuccessfullySent(count int) {
	m.count.Add(int64(count))
}
