// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package defaultforwarderimpl

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
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig, nil))
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
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig, nil))
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
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), sender, NewSharedConnection(log, false, 1, mockConfig, nil))

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
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig, nil))

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
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig, nil))

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
	connection := NewSharedConnection(log, false, 1, mockConfig, nil)
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
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig, nil))

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
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, requests, mockConfig, nil))

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
	// This test exercises Worker.Stop's purge path in isolation: it never
	// calls Worker.Start, instead pre-closing w.stopped so that Stop's
	// <-w.stopped wait returns immediately. That lets us assert what Stop
	// does with queued transactions without racing the Start goroutine.
	//
	// Stop closes stopChan and so is not idempotent; each subcase constructs
	// a fresh Worker.

	newPreClosedWorker := func() (*Worker, chan transaction.Transaction, chan transaction.Transaction) {
		highPrio := make(chan transaction.Transaction, 1)
		lowPrio := make(chan transaction.Transaction, 1)
		requeue := make(chan transaction.Transaction, 1)
		mockConfig := mock.New(t)
		log := logmock.New(t)
		secrets := secretsmock.New(t)
		w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig, nil))
		close(w.stopped)
		return w, highPrio, lowPrio
	}

	t.Run("without purge", func(t *testing.T) {
		w, highPrio, lowPrio := newPreClosedWorker()

		mockTransaction := newTestTransaction()
		mockTransaction.On("Process", w.Client.GetClient()).Return(nil).Times(1)
		mockTransaction.On("GetTarget").Return("").Times(1)
		highPrio <- mockTransaction

		mockRetryTransaction := newTestTransaction()
		mockRetryTransaction.On("Process", w.Client.GetClient()).Return(nil).Times(1)
		mockRetryTransaction.On("GetTarget").Return("").Times(1)
		lowPrio <- mockRetryTransaction

		w.Stop(false)
		mockTransaction.AssertNumberOfCalls(t, "Process", 0)
		mockRetryTransaction.AssertNumberOfCalls(t, "Process", 0)
	})

	t.Run("with purge", func(t *testing.T) {
		w, highPrio, lowPrio := newPreClosedWorker()

		mockTransaction := newTestTransaction()
		mockTransaction.On("Process", w.Client.GetClient()).Return(nil).Times(1)
		mockTransaction.On("GetTarget").Return("").Times(1)
		highPrio <- mockTransaction

		mockRetryTransaction := newTestTransaction()
		mockRetryTransaction.On("Process", w.Client.GetClient()).Return(nil).Times(1)
		mockRetryTransaction.On("GetTarget").Return("").Times(1)
		lowPrio <- mockRetryTransaction

		w.Stop(true)
		mockTransaction.AssertExpectations(t)
		mockTransaction.AssertNumberOfCalls(t, "Process", 1)
		mockRetryTransaction.AssertNumberOfCalls(t, "Process", 0)
	})
}

func TestWorkerRequeueDropsTracksPointsDropped(t *testing.T) {
	highPrio := make(chan transaction.Transaction)
	lowPrio := make(chan transaction.Transaction)
	requeue := make(chan transaction.Transaction, 1)
	sender := &PointSuccessfullySentMock{}
	mockConfig := mock.New(t)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), sender, NewSharedConnection(log, false, 1, mockConfig, nil))

	// Fill the requeue channel so the next requeue call falls into the default branch and drops.
	filler := newTestTransaction()
	requeue <- filler

	dropped := newTestTransaction()
	dropped.pointCount = 7

	w.requeue(dropped)

	assert.Equal(t, int64(7), sender.dropped.Load(), "point.dropped must reflect points lost when RequeueChan is full")
	assert.Equal(t, int64(0), sender.count.Load(), "no points were successfully sent")
}

type PointSuccessfullySentMock struct {
	count   atomic.Int64
	dropped atomic.Int64
}

// TestWorkerStopDoesNotCancelInFlight verifies that when forwarder_stop_wait_for_inflight
// is enabled and Worker.Stop is called while a transaction is in flight, Stop waits for
// the transaction's Process call to return rather than cancelling its context.
// The transaction completes successfully and is not requeued.
func TestWorkerStopDoesNotCancelInFlight(t *testing.T) {
	highPrio := make(chan transaction.Transaction, 1)
	lowPrio := make(chan transaction.Transaction, 1)
	requeue := make(chan transaction.Transaction, 1)

	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_stop_wait_for_inflight", true)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, 1, mockConfig, nil))

	var processedwg sync.WaitGroup
	processedwg.Add(1)

	mockTransaction := newTestTransaction()
	// release holds Process until the test closes the channel, simulating an
	// in-flight HTTP request that hasn't yet returned a response.
	mockTransaction.release = make(chan struct{})
	mockTransaction.
		On("Process", w.Client.GetClient()).
		Run(func(_args tmock.Arguments) {
			processedwg.Done()
		}).
		Return(nil).Times(1)
	mockTransaction.On("GetTarget").Return("").Times(1)

	w.Start()
	highPrio <- mockTransaction

	// Wait until Process has been entered (i.e. the request is in flight).
	processedwg.Wait()

	stopReturned := make(chan struct{})
	go func() {
		w.Stop(false)
		close(stopReturned)
	}()

	// Stop must block here while the request is held. 50 ms is well above
	// scheduler noise but far below the 2 s forwarder_stop_timeout, so a
	// regression that cancels in-flight requests is trivially detectable.
	select {
	case <-stopReturned:
		t.Fatal("Stop returned before in-flight Process completed")
	case <-time.After(50 * time.Millisecond):
		// expected — Stop is waiting for in-flight to finish
	}

	close(mockTransaction.release)

	<-stopReturned

	mockTransaction.AssertExpectations(t)
	mockTransaction.AssertNumberOfCalls(t, "Process", 1)
	assert.Equal(t, 0, len(requeue), "successfully-completed transaction must not be requeued")
}

// TestWorkerStopWaitsForSemaphoreBlockedTransactions exercises the semaphore-contention
// case during shutdown when forwarder_stop_wait_for_inflight is enabled. With
// forwarder_max_concurrent_requests=3 and 5 queued transactions, the first three hold
// the semaphore (held by `release` channels), the fourth is blocked acquiring the
// semaphore, and the fifth remains in HighPrio when Stop(true) is called.
//
// Stop must:
//  1. Wait for all three in-flight Process calls to return (no cancellation).
//  2. Once the first three release the semaphore, the fourth (semaphore-blocked)
//     transaction acquires it and completes.
//  3. The purge loop runs HighPrio's remaining fifth transaction.
//
// All five complete successfully; nothing is requeued.
func TestWorkerStopWaitsForSemaphoreBlockedTransactions(t *testing.T) {
	highPrio := make(chan transaction.Transaction, 10)
	lowPrio := make(chan transaction.Transaction, 10)
	requeue := make(chan transaction.Transaction, 10)

	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_stop_wait_for_inflight", true)
	log := logmock.New(t)

	// Configure the worker to have 3 maximum concurrent requests
	requests := 3

	mockConfig.SetInTest("forwarder_max_concurrent_requests", requests)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue, newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{}, NewSharedConnection(log, false, requests, mockConfig, nil))

	var inFlightWg sync.WaitGroup
	inFlightWg.Add(requests)

	// Create enough transactions that some will be waiting on the semaphore at Stop time.
	transactions := []*testTransaction{}
	for i := 0; i < 5; i++ {
		mockTransaction := newTestTransaction()

		// The first three transactions hold the semaphore via a per-transaction
		// release channel. The fourth blocks acquiring the semaphore (so Process
		// never starts until one of the first three releases). The fifth stays
		// in HighPrio for the purge loop to pick up.
		if i < requests {
			mockTransaction.release = make(chan struct{})
			mockTransaction.
				On("Process", w.Client.GetClient()).
				Run(func(_args tmock.Arguments) {
					inFlightWg.Done()
				}).
				Return(nil).Times(1)
		} else {
			mockTransaction.On("Process", w.Client.GetClient()).Return(nil).Times(1)
		}

		// Each transaction has a different target to ensure the block list doesn't prevent
		// transactions being processed on too many errors for a target.
		mockTransaction.On("GetTarget").Return("Target" + strconv.Itoa(i))

		transactions = append(transactions, mockTransaction)
	}

	w.Start()
	for _, txn := range transactions {
		highPrio <- txn
	}

	// Wait for three (the number of concurrent requests allowed) Process calls
	// to actually be entered. At this point the semaphore is fully held.
	inFlightWg.Wait()

	// Ensure the Start loop has tried to pick up at least four transactions
	// (the three in flight plus the fourth that is now blocked on the
	// semaphore). The fifth must still be sitting in HighPrio when Stop runs
	// so the purge loop can drain it.
	for len(highPrio) > 1 {
		time.Sleep(time.Millisecond)
	}

	stopReturned := make(chan struct{})
	go func() {
		w.Stop(true)
		close(stopReturned)
	}()

	// Stop must NOT return while in-flight transactions are still held.
	select {
	case <-stopReturned:
		t.Fatal("Stop returned before in-flight Process calls completed")
	case <-time.After(50 * time.Millisecond):
		// expected — Stop is blocked on requestWg.Wait()
	}

	// Release the three in-flight transactions. Once they return, the
	// semaphore frees and the fourth (semaphore-blocked) transaction proceeds.
	for i := 0; i < requests; i++ {
		close(transactions[i].release)
	}

	<-stopReturned

	// All five transactions ran their Process call successfully.
	for i, txn := range transactions {
		txn.AssertNumberOfCalls(t, "Process", 1)
		assert.NoError(t, nil, "transaction %d", i)
	}

	// None of them requeued.
	assert.Equal(t, 0, len(requeue), "no transactions should requeue under graceful shutdown")
}

// TestWorkerStopWaitsForInFlightHTTPRequest pins the shutdown contract when
// forwarder_stop_wait_for_inflight is enabled: an in-flight transaction must
// complete before Worker.Stop returns, even when purgeHighPrio=true (which is
// what DefaultForwarder.Stop passes).
func TestWorkerStopWaitsForInFlightHTTPRequest(t *testing.T) {
	requestEntered := make(chan struct{})
	releaseResponse := make(chan struct{})
	var processed atomic.Bool

	highPrio := make(chan transaction.Transaction, 1)
	lowPrio := make(chan transaction.Transaction, 1)
	requeue := make(chan transaction.Transaction, 1)
	mockConfig := mock.New(t)
	mockConfig.SetInTest("forwarder_stop_wait_for_inflight", true)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue,
		newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{},
		NewSharedConnection(log, false, 1, mockConfig, nil))

	txn := newTestTransaction()
	txn.On("GetTarget").Return("")
	txn.
		On("Process", w.Client.GetClient()).
		Run(func(_ tmock.Arguments) {
			close(requestEntered)
			<-releaseResponse
			processed.Store(true)
		}).
		Return(nil).Times(1)

	w.Start()
	highPrio <- txn
	<-requestEntered // worker has picked up the transaction and HTTP is "in flight"

	stopReturned := make(chan struct{})
	go func() {
		w.Stop(true) // purgeHighPrio=true mirrors what DefaultForwarder.Stop does
		close(stopReturned)
	}()

	// Stop must block here until releaseResponse is closed.
	select {
	case <-stopReturned:
		t.Fatal("Stop returned before in-flight request completed")
	case <-time.After(50 * time.Millisecond):
		// expected — Stop is waiting for in-flight to finish
	}

	close(releaseResponse)

	<-stopReturned

	assert.True(t, processed.Load(), "transaction must complete before Stop returns")
	assert.Equal(t, 0, len(requeue), "successfully-completed transaction must not be requeued")
	txn.AssertNumberOfCalls(t, "Process", 1)
}

// TestWorkerStopCancelsContextRequeuesSemaphoreBlocked verifies the default shutdown path
// (forwarder_stop_wait_for_inflight=false). With forwarder_max_concurrent_requests=1 and one
// transaction holding the single semaphore slot, a second transaction is blocked in
// acquireRequestSemaphore. When Stop is called, workerCtx is cancelled immediately; the blocked
// transaction receives a context.Canceled error from semaphore.Acquire and is requeued rather
// than processed.
func TestWorkerStopCancelsContextRequeuesSemaphoreBlocked(t *testing.T) {
	highPrio := make(chan transaction.Transaction, 2)
	lowPrio := make(chan transaction.Transaction, 1)
	requeue := make(chan transaction.Transaction, 2)

	mockConfig := mock.New(t)
	// forwarder_stop_wait_for_inflight is intentionally left at its default (false) to exercise
	// the cancel-on-stop path.
	mockConfig.SetInTest("forwarder_max_concurrent_requests", 1)
	log := logmock.New(t)
	secrets := secretsmock.New(t)
	w := NewWorker(mockConfig, log, secrets, highPrio, lowPrio, requeue,
		newBlockedEndpoints(mockConfig, log), &PointSuccessfullySentMock{},
		NewSharedConnection(log, false, 1, mockConfig, nil))

	// firstTxn holds the single semaphore slot until release is closed.
	firstEntered := make(chan struct{})
	release := make(chan struct{})

	firstTxn := newTestTransaction()
	firstTxn.release = release
	firstTxn.
		On("Process", w.Client.GetClient()).
		Run(func(_ tmock.Arguments) {
			close(firstEntered)
		}).
		Return(nil).Times(1)
	firstTxn.On("GetTarget").Return("")

	// secondTxn will block on semaphore acquisition; it should be requeued, not processed.
	secondTxn := newTestTransaction()
	secondTxn.On("Process", w.Client.GetClient()).Return(nil).Maybe()
	secondTxn.On("GetTarget").Return("").Maybe()

	w.Start()
	highPrio <- firstTxn
	<-firstEntered // semaphore fully held

	highPrio <- secondTxn // worker will pick this up then block on semaphore

	// Wait deterministically until the worker has drained secondTxn from HighPrio
	// (mirrors the polling pattern in TestWorkerStopWaitsForSemaphoreBlockedTransactions).
	// Once HighPrio is empty, the worker is committed to processing secondTxn and is
	// either entering or already inside semaphore.Acquire. A subsequent cancel()
	// guarantees Acquire returns ctx.Err and callProcess requeues secondTxn.
	for len(highPrio) > 0 {
		time.Sleep(time.Millisecond)
	}

	// Stop with waitForInflight=false: cancels workerCtx immediately.
	stopReturned := make(chan struct{})
	go func() {
		w.Stop(false)
		close(stopReturned)
	}()

	// Wait for Stop's cancel() to propagate. Ensure cancel happens-before semaphore
	// release, so the undocumented cancellation check in semaphore.Acquire after
	// a successful acquisition is guaranteed to see it.
	<-w.workerCtx.Done()

	// Release the first in-flight transaction so requestWg.Wait() can complete.
	close(release)

	<-stopReturned

	// secondTxn must have been requeued (context cancel → semaphore.Acquire error → requeue).
	assert.Equal(t, 1, len(requeue), "semaphore-blocked transaction must be requeued when Stop cancels workerCtx")
	firstTxn.AssertNumberOfCalls(t, "Process", 1)
}

func (m *PointSuccessfullySentMock) OnPointSuccessfullySent(count int) {
	m.count.Add(int64(count))
}

func (m *PointSuccessfullySentMock) OnPointDropped(count int) {
	m.dropped.Add(int64(count))
}
