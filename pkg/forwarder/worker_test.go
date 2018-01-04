// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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

	config.Datadog.Set("skip_ssl_validation", true)
	defer config.Datadog.Set("skip_ssl_validation", false)

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

	dummy := newTestTransaction()
	dummy.On("Process", w.Client).Return(nil).Times(2)
	dummy.On("GetTarget").Return("").Times(2)

	w.Start()

	highPrio <- mock
	// since highPrio and lowPrio have no buffering the worker won't take another Transaction until it has processed the first one
	highPrio <- dummy

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)
	mock.AssertNumberOfCalls(t, "Reschedule", 0)

	lowPrio <- mock2
	lowPrio <- dummy

	mock2.AssertExpectations(t)
	mock2.AssertNumberOfCalls(t, "Process", 1)
	mock2.AssertNumberOfCalls(t, "Reschedule", 0)

	w.Stop()
}

func TestWorkerRetry(t *testing.T) {
	highPrio := make(chan Transaction)
	lowPrio := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(highPrio, lowPrio, requeue, newBlockedEndpoints())

	mock := newTestTransaction()
	mock.On("Process", w.Client).Return(fmt.Errorf("some kind of error")).Times(1)
	mock.On("Reschedule").Return(nil).Times(1)
	mock.On("GetTarget").Return("error_url").Times(1)

	w.Start()
	highPrio <- mock
	retryTransaction := <-requeue
	w.Stop()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)
	mock.AssertNumberOfCalls(t, "GetTarget", 1)
	mock.AssertNumberOfCalls(t, "Reschedule", 1)
	assert.Equal(t, mock, retryTransaction)
	assert.True(t, w.blockedList.isBlock("error_url"))
}

func TestWorkerRetryBlockedTransaction(t *testing.T) {
	highPrio := make(chan Transaction)
	lowPrio := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(highPrio, lowPrio, requeue, newBlockedEndpoints())

	mock := newTestTransaction()
	mock.On("Reschedule").Return(nil).Times(1)
	mock.On("GetTarget").Return("error_url").Times(1)

	w.blockedList.block("error_url")
	w.Start()
	highPrio <- mock
	retryTransaction := <-requeue
	w.Stop()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 0)
	mock.AssertNumberOfCalls(t, "GetTarget", 1)
	mock.AssertNumberOfCalls(t, "Reschedule", 1)
	assert.Equal(t, mock, retryTransaction)
	assert.True(t, w.blockedList.isBlock("error_url"))
}
