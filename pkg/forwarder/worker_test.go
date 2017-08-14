// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

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
	input := make(chan Transaction)
	requeue := make(chan Transaction)

	w := NewWorker(input, requeue, newBlockedEndpoints())
	assert.NotNil(t, w)
	assert.Equal(t, w.Client.Timeout, config.Datadog.GetDuration("forwarder_timeout")*time.Second)
}

func TestNewNoSSLWorker(t *testing.T) {
	input := make(chan Transaction)
	requeue := make(chan Transaction)

	config.Datadog.Set("skip_ssl_validation", true)
	defer config.Datadog.Set("skip_ssl_validation", false)

	w := NewWorker(input, requeue, newBlockedEndpoints())
	assert.True(t, w.Client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
}

func TestWorkerStart(t *testing.T) {
	input := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(input, requeue, newBlockedEndpoints())

	mock := newTestTransaction()
	mock.On("Process", w.Client).Return(nil).Times(1)
	mock.On("GetTarget").Return("").Times(1)

	dummy := newTestTransaction()
	dummy.On("Process", w.Client).Return(nil).Times(1)
	dummy.On("GetTarget").Return("").Times(1)

	w.Start()
	input <- mock
	// since input has no buffering the worker won't take another Transaction until it has processed the first one
	input <- dummy
	w.Stop()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 1)
	mock.AssertNumberOfCalls(t, "Reschedule", 0)
}

func TestWorkerRetry(t *testing.T) {
	input := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(input, requeue, newBlockedEndpoints())

	mock := newTestTransaction()
	mock.On("Process", w.Client).Return(fmt.Errorf("some kind of error")).Times(1)
	mock.On("Reschedule").Return(nil).Times(1)
	mock.On("GetTarget").Return("error_url").Times(1)

	w.Start()
	input <- mock
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
	input := make(chan Transaction)
	requeue := make(chan Transaction, 1)
	w := NewWorker(input, requeue, newBlockedEndpoints())

	mock := newTestTransaction()
	mock.On("Reschedule").Return(nil).Times(1)
	mock.On("GetTarget").Return("error_url").Times(1)

	w.blockedList.block("error_url")
	w.Start()
	input <- mock
	retryTransaction := <-requeue
	w.Stop()
	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Process", 0)
	mock.AssertNumberOfCalls(t, "GetTarget", 1)
	mock.AssertNumberOfCalls(t, "Reschedule", 1)
	assert.Equal(t, mock, retryTransaction)
}
