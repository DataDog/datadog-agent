// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build !windows

package forwarder

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	keysPerDomains = map[string][]string{
		"datadog.foo": {"api-key-1", "api-key-2"},
		"datadog.bar": nil,
	}
)

func TestNewDefaultForwarder(t *testing.T) {
	forwarder := NewDefaultForwarder(keysPerDomains)

	assert.NotNil(t, forwarder)
	assert.Equal(t, forwarder.NumberOfWorkers, 4)
	assert.Equal(t, forwarder.KeysPerDomains, keysPerDomains)

	assert.Nil(t, forwarder.highPrio)
	assert.Nil(t, forwarder.lowPrio)
	assert.Nil(t, forwarder.requeuedTransaction)
	assert.Nil(t, forwarder.stopRetry)
	assert.Len(t, forwarder.workers, 0)
	assert.Len(t, forwarder.retryQueue, 0)
	assert.Equal(t, forwarder.internalState, Stopped)
	assert.Equal(t, forwarder.State(), forwarder.internalState)
}

func TestStart(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)
	err := forwarder.Start()

	assert.Nil(t, err)
	require.Len(t, forwarder.retryQueue, 0)
	assert.Equal(t, Started, forwarder.State())
	assert.NotNil(t, forwarder.highPrio)
	assert.NotNil(t, forwarder.lowPrio)
	assert.NotNil(t, forwarder.requeuedTransaction)
	assert.NotNil(t, forwarder.stopRetry)
	assert.NotNil(t, forwarder.Start())

	forwarder.Stop()
}

func TestInit(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)
	forwarder.init()
	assert.Len(t, forwarder.workers, 0)
	assert.Len(t, forwarder.retryQueue, 0)
}

func TestStop(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)
	forwarder.Stop() // this should be a noop
	assert.Equal(t, Stopped, forwarder.State())
	forwarder.Start()
	forwarder.Stop()
	assert.Len(t, forwarder.workers, 0)
	assert.Len(t, forwarder.retryQueue, 0)
	assert.Equal(t, Stopped, forwarder.State())
}

func TestSubmitIfStopped(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)

	require.NotNil(t, forwarder)
	require.Equal(t, Stopped, forwarder.State())
	assert.NotNil(t, forwarder.SubmitSeries(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitEvents(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitServiceChecks(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitSketchSeries(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitHostMetadata(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitMetadata(nil, make(http.Header)))
}

func TestCreateHTTPTransactions(t *testing.T) {
	forwarder := NewDefaultForwarder(keysPerDomains)
	endpoint := "/api/foo"
	p1 := []byte("A payload")
	p2 := []byte("Another payload")
	payloads := Payloads{&p1, &p2}
	headers := make(http.Header)
	headers.Set("HTTP-MAGIC", "foo")

	transactions := forwarder.createHTTPTransactions(endpoint, payloads, false, headers)
	require.Len(t, transactions, 4)
	assert.Equal(t, "datadog.foo", transactions[0].Domain)
	assert.Equal(t, "datadog.foo", transactions[1].Domain)
	assert.Equal(t, "datadog.foo", transactions[2].Domain)
	assert.Equal(t, "datadog.foo", transactions[3].Domain)
	assert.Equal(t, endpoint, transactions[0].Endpoint)
	assert.Equal(t, endpoint, transactions[1].Endpoint)
	assert.Equal(t, endpoint, transactions[2].Endpoint)
	assert.Equal(t, endpoint, transactions[3].Endpoint)
	assert.Len(t, transactions[0].Headers, 3)
	assert.NotEmpty(t, transactions[0].Headers.Get("DD-Api-Key"))
	assert.NotEmpty(t, transactions[0].Headers.Get("HTTP-MAGIC"))
	assert.Equal(t, version.AgentVersion, transactions[0].Headers.Get("DD-Agent-Version"))
	assert.Equal(t, p1, *(transactions[0].Payload))
	assert.Equal(t, p1, *(transactions[1].Payload))
	assert.Equal(t, p2, *(transactions[2].Payload))
	assert.Equal(t, p2, *(transactions[3].Payload))

	transactions = forwarder.createHTTPTransactions(endpoint, payloads, true, headers)
	require.Len(t, transactions, 4)
	assert.Contains(t, transactions[0].Endpoint, "api_key=api-key-1")
	assert.Contains(t, transactions[1].Endpoint, "api_key=api-key-2")
	assert.Contains(t, transactions[2].Endpoint, "api_key=api-key-1")
	assert.Contains(t, transactions[3].Endpoint, "api_key=api-key-2")
}

func TestSendHTTPTransactions(t *testing.T) {
	forwarder := NewDefaultForwarder(keysPerDomains)
	endpoint := "/api/foo"
	p1 := []byte("A payload")
	payloads := Payloads{&p1}
	headers := make(http.Header)
	tr := forwarder.createHTTPTransactions(endpoint, payloads, false, headers)

	// fw is stopped, we should get an error
	err := forwarder.sendHTTPTransactions(tr)
	assert.NotNil(t, err)

	forwarder.Start()
	err = forwarder.sendHTTPTransactions(tr)
	assert.Nil(t, err)
}

func TestRequeueTransaction(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)
	tr := NewHTTPTransaction()
	assert.Len(t, forwarder.retryQueue, 0)
	forwarder.requeueTransaction(tr)
	assert.Len(t, forwarder.retryQueue, 1)
}

func TestRetryTransactions(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)
	forwarder.init()
	forwarder.retryQueueLimit = 1

	t1 := NewHTTPTransaction()
	t1.nextFlush = time.Now().Add(-1 * time.Hour)
	t2 := NewHTTPTransaction()
	t2.nextFlush = time.Now().Add(1 * time.Hour)
	forwarder.requeueTransaction(t2)
	forwarder.requeueTransaction(t2) // this second one should be dropped
	forwarder.requeueTransaction(t1) // the queue should be sorted
	forwarder.retryTransactions(time.Now())
	assert.Len(t, forwarder.retryQueue, 1)
	assert.Len(t, forwarder.lowPrio, 1)
	dropped, _ := strconv.ParseInt(transactionsExpvar.Get("Dropped").String(), 10, 64)
	assert.Equal(t, int64(1), dropped)
}

func TestForwarderRetry(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)
	forwarder.Start()
	defer forwarder.Stop()

	ready := newTestTransaction()
	notReady := newTestTransaction()

	forwarder.requeueTransaction(ready)
	forwarder.requeueTransaction(notReady)
	require.Len(t, forwarder.retryQueue, 2)

	ready.On("Process", forwarder.workers[0].Client).Return(nil).Times(1)
	ready.On("GetTarget").Return("").Times(1)
	ready.On("GetNextFlush").Return(time.Now()).Times(1)
	ready.On("GetCreatedAt").Return(time.Now()).Times(1)
	notReady.On("GetNextFlush").Return(time.Now().Add(10 * time.Minute)).Times(1)
	notReady.On("GetCreatedAt").Return(time.Now()).Times(1)

	forwarder.retryTransactions(time.Now())
	<-ready.processed

	ready.AssertExpectations(t)
	notReady.AssertExpectations(t)
	notReady.AssertNumberOfCalls(t, "Process", 0)
	notReady.AssertNumberOfCalls(t, "GetTarget", 0)
	require.Len(t, forwarder.retryQueue, 1)
	assert.Equal(t, forwarder.retryQueue[0], notReady)
}

func TestForwarderRetryLifo(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)
	forwarder.init()

	transaction1 := newTestTransaction()
	transaction2 := newTestTransaction()

	forwarder.requeueTransaction(transaction1)
	forwarder.requeueTransaction(transaction2)

	transaction1.On("GetNextFlush").Return(time.Now()).Times(1)
	transaction1.On("GetCreatedAt").Return(time.Now()).Times(1)

	transaction2.On("GetNextFlush").Return(time.Now()).Times(1)
	transaction2.On("GetCreatedAt").Return(time.Now().Add(1 * time.Minute)).Times(1)

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
	forwarder := NewDefaultForwarder(nil)
	forwarder.init()

	forwarder.retryQueueLimit = 1

	transaction1 := newTestTransaction()
	transaction2 := newTestTransaction()

	forwarder.requeueTransaction(transaction1)
	forwarder.requeueTransaction(transaction2)

	transaction1.On("GetNextFlush").Return(time.Now().Add(1 * time.Minute)).Times(1)
	transaction1.On("GetCreatedAt").Return(time.Now()).Times(1)

	transaction2.On("GetNextFlush").Return(time.Now().Add(1 * time.Minute)).Times(1)
	transaction2.On("GetCreatedAt").Return(time.Now().Add(1 * time.Minute)).Times(1)

	forwarder.retryTransactions(time.Now())

	transaction1.AssertExpectations(t)
	transaction2.AssertExpectations(t)
	require.Len(t, forwarder.retryQueue, 1)
	require.Len(t, forwarder.highPrio, 0)
	require.Len(t, forwarder.lowPrio, 0)
	// assert that the oldest transaction was dropped
	assert.Equal(t, transaction2, forwarder.retryQueue[0])
}
