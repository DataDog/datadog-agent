// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestNewWorker(t *testing.T) {
	highPrio := make(chan Transaction)
	lowPrio := make(chan Transaction)
	requeue := make(chan Transaction)

	w := NewWorker(highPrio, lowPrio, requeue, newBlockedEndpoints())
	assert.NotNil(t, w)
	assert.Equal(t, w.Client.Timeout, config.Datadog.GetDuration("forwarder_timeout")*time.Second)
}

func TestNewNoSSLWorker(t *testing.T) {
	highPrio := make(chan Transaction)
	lowPrio := make(chan Transaction)
	requeue := make(chan Transaction)

	mockConfig := config.Mock()
	mockConfig.Set("skip_ssl_validation", true)
	defer mockConfig.Set("skip_ssl_validation", false)

	w := NewWorker(highPrio, lowPrio, requeue, newBlockedEndpoints())
	assert.True(t, w.Client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
}

func TestWorkerStart(t *testing.T) {
	highPrio := make(chan Transaction)
	lowPrio := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(highPrio, lowPrio, requeue, newBlockedEndpoints())

	mock := newTestTransaction()
	mock.On("Process", w.Client).Return(nil).Times(1)
	mock.On("GetTarget").Return("").Times(1)
	mock2 := newTestTransaction()
	mock2.On("Process", w.Client).Return(nil).Times(1)
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

	w.Stop(false)
}

func TestWorkerRetry(t *testing.T) {
	highPrio := make(chan Transaction)
	lowPrio := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(highPrio, lowPrio, requeue, newBlockedEndpoints())

	mock := newTestTransaction()
	mock.On("Process", w.Client).Return(fmt.Errorf("some kind of error")).Times(1)
	mock.On("GetTarget").Return("error_url").Times(1)

	w.Start()
	highPrio <- mock
	retryTransaction := <-requeue
	w.Stop(false)
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)
	mock.AssertNumberOfCalls(t, "GetTarget", 1)
	assert.Equal(t, mock, retryTransaction)
	assert.True(t, w.blockedList.isBlock("error_url"))
}

func TestWorkerRetryBlockedTransaction(t *testing.T) {
	highPrio := make(chan Transaction)
	lowPrio := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(highPrio, lowPrio, requeue, newBlockedEndpoints())

	mock := newTestTransaction()
	mock.On("GetTarget").Return("error_url").Times(1)

	w.blockedList.close("error_url")
	w.Start()
	highPrio <- mock
	retryTransaction := <-requeue
	w.Stop(false)
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 0)
	mock.AssertNumberOfCalls(t, "GetTarget", 1)
	assert.Equal(t, mock, retryTransaction)
	assert.True(t, w.blockedList.isBlock("error_url"))
}

func TestWorkerResetConnections(t *testing.T) {
	highPrio := make(chan Transaction)
	lowPrio := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(highPrio, lowPrio, requeue, newBlockedEndpoints())

	mock := newTestTransaction()
	mock.On("Process", w.Client).Return(nil).Times(1)
	mock.On("GetTarget").Return("").Times(1)

	w.Start()

	highPrio <- mock
	<-mock.processed

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)

	httpClientBefore := w.Client
	w.ScheduleConnectionReset()

	// tricky to test here that "Process" is called with a new http client
	mock2 := newTestTransactionWithoutClientAssert()
	mock2.On("Process").Return(nil).Times(1)
	mock2.On("GetTarget").Return("").Times(1)
	highPrio <- mock2
	<-mock2.processed
	mock2.AssertExpectations(t)
	mock2.AssertNumberOfCalls(t, "Process", 1)

	assert.NotSame(t, httpClientBefore, w.Client)

	mock3 := newTestTransaction()
	mock3.On("Process", w.Client).Return(nil).Times(1)
	mock3.On("GetTarget").Return("").Times(1)
	highPrio <- mock3
	<-mock3.processed
	mock3.AssertExpectations(t)
	mock3.AssertNumberOfCalls(t, "Process", 1)

	w.Stop(false)
}

func TestWorkerPurgeOnStop(t *testing.T) {
	highPrio := make(chan Transaction, 1)
	lowPrio := make(chan Transaction, 1)
	requeue := make(chan Transaction, 1)
	w := NewWorker(highPrio, lowPrio, requeue, newBlockedEndpoints())
	// making stopChan non blocking on insert and closing stopped channel
	// to avoid blocking in the Stop method since we don't actually start
	// the workder
	w.stopChan = make(chan struct{}, 2)
	close(w.stopped)

	mockTransaction := newTestTransaction()
	mockTransaction.On("Process", w.Client).Return(nil).Times(1)
	mockTransaction.On("GetTarget").Return("").Times(1)
	highPrio <- mockTransaction

	mockRetryTransaction := newTestTransaction()
	mockRetryTransaction.On("Process", w.Client).Return(nil).Times(1)
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
