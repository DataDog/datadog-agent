// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package forwarder

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestNewDefaultForwarder(t *testing.T) {
	keysPerDomains := map[string][]string{"domainsA": []string{"key1", "key2"}, "domainsB": nil}
	forwarder := NewDefaultForwarder(keysPerDomains)

	assert.NotNil(t, forwarder)
	assert.Equal(t, forwarder.NumberOfWorkers, 4)
	assert.Equal(t, forwarder.KeysPerDomains, keysPerDomains)

	assert.Nil(t, forwarder.waitingPipe)
	assert.Nil(t, forwarder.requeuedTransaction)
	assert.Nil(t, forwarder.stopRetry)
	assert.Len(t, forwarder.workers, 0)
	assert.Len(t, forwarder.retryQueue, 0)
	assert.Equal(t, forwarder.internalState, Stopped)
}

func TestState(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)

	assert.NotNil(t, forwarder)
	assert.Equal(t, forwarder.State(), Stopped)
	forwarder.internalState = Started
	assert.Equal(t, forwarder.State(), Started)
}

func TestForwarderStart(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)

	err := forwarder.Start()
	defer forwarder.Stop()
	assert.Nil(t, err)

	require.Len(t, forwarder.retryQueue, 0)
	assert.Equal(t, forwarder.internalState, Started)
	assert.NotNil(t, forwarder.waitingPipe)
	assert.NotNil(t, forwarder.requeuedTransaction)
	assert.NotNil(t, forwarder.stopRetry)

	assert.NotNil(t, forwarder.Start())
}

func TestSubmitInStopMode(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)

	assert.NotNil(t, forwarder)
	assert.NotNil(t, forwarder.SubmitSeries(nil, map[string]string{}))
	assert.NotNil(t, forwarder.SubmitEvents(nil, map[string]string{}))
	assert.NotNil(t, forwarder.SubmitServiceChecks(nil, map[string]string{}))
	assert.NotNil(t, forwarder.SubmitSketchSeries(nil, map[string]string{}))
	assert.NotNil(t, forwarder.SubmitHostMetadata(nil, map[string]string{}))
	assert.NotNil(t, forwarder.SubmitMetadata(nil, map[string]string{}))
}

func TestSubmit(t *testing.T) {
	expectedEndpoint := ""
	expectAPIKeyInQuery := false
	expectedPayload := []byte{}
	expectedHeaders := make(http.Header)
	var payloads []*[]byte

	firstKey := "api_key1"
	secondKey := "api_key2"
	currentKey := firstKey
	expectedHeaders.Set(apiHTTPHeaderKey, firstKey)
	expectedHeaders.Set("Content-Type", "application/unit-test")

	wait := make(chan bool)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { wait <- true }()
		assert.Equal(t, r.Method, "POST")
		assert.Equal(t, r.URL.Path, expectedEndpoint)
		if expectAPIKeyInQuery {
			assert.Equal(t, r.URL.RawQuery, fmt.Sprintf("api_key=%s", currentKey))
		}
		for k := range expectedHeaders {
			assert.Equal(t, expectedHeaders.Get(k), r.Header.Get(k))
		}

		// We switch expected keys as the forwarder should create one
		// transaction per keys
		if expectedHeaders.Get(apiHTTPHeaderKey) == firstKey {
			currentKey = secondKey
			expectedHeaders.Set(apiHTTPHeaderKey, secondKey)
		} else {
			currentKey = firstKey
			expectedHeaders.Set(apiHTTPHeaderKey, firstKey)
		}

		body, err := ioutil.ReadAll(r.Body)
		assert.Nil(t, err)
		assert.Equal(t, body, expectedPayload)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	forwarder := NewDefaultForwarder(map[string][]string{ts.URL: []string{firstKey, secondKey}})
	// delay next retry cycle so we can inspect retry queue
	flushInterval = 5 * time.Hour
	forwarder.NumberOfWorkers = 1
	forwarder.Start()
	defer forwarder.Stop()

	expectedPayload = []byte("SubmitSeries payload")
	expectedEndpoint = "/api/v2/series"
	payloads = []*[]byte{}
	payloads = append(payloads, &expectedPayload)
	assert.Nil(t, forwarder.SubmitSeries(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	// wait for the queries to complete before changing expected value
	<-wait
	<-wait
	require.Len(t, forwarder.retryQueue, 0)

	expectedPayload = []byte("SubmitEvents payload")
	expectedEndpoint = "/api/v2/events"
	payloads = []*[]byte{}
	payloads = append(payloads, &expectedPayload)
	assert.Nil(t, forwarder.SubmitEvents(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	<-wait
	<-wait
	require.Len(t, forwarder.retryQueue, 0)

	expectedPayload = []byte("SubmitServiceChecks payload")
	expectedEndpoint = "/api/v2/service_checks"
	payloads = []*[]byte{}
	payloads = append(payloads, &expectedPayload)
	assert.Nil(t, forwarder.SubmitServiceChecks(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	<-wait
	<-wait
	require.Len(t, forwarder.retryQueue, 0)

	expectedPayload = []byte("SubmitSketchSeries payload")
	expectedEndpoint = "/api/beta/sketches"
	expectAPIKeyInQuery = true
	payloads = []*[]byte{}
	payloads = append(payloads, &expectedPayload)
	assert.Nil(t, forwarder.SubmitSketchSeries(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	<-wait
	<-wait
	require.Len(t, forwarder.retryQueue, 0)
	expectAPIKeyInQuery = false

	expectedPayload = []byte("SubmitHostMetadata payload")
	expectedEndpoint = "/api/v2/host_metadata"
	payloads = []*[]byte{}
	payloads = append(payloads, &expectedPayload)
	assert.Nil(t, forwarder.SubmitHostMetadata(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	<-wait
	<-wait
	require.Len(t, forwarder.retryQueue, 0)

	expectedPayload = []byte("SubmitMetadata payload")
	expectedEndpoint = "/api/v2/metadata"
	payloads = []*[]byte{&expectedPayload}
	assert.Nil(t, forwarder.SubmitMetadata(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	<-wait
	<-wait
	require.Len(t, forwarder.retryQueue, 0)

	expectedPayload = []byte("SubmitV1Series payload")
	expectedEndpoint = "/api/v1/series"
	expectAPIKeyInQuery = true
	payloads = []*[]byte{}
	payloads = append(payloads, &expectedPayload)
	assert.Nil(t, forwarder.SubmitV1Series(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	<-wait
	<-wait
	require.Len(t, forwarder.retryQueue, 0)

	expectedPayload = []byte("SubmitV1CheckRuns payload")
	expectedEndpoint = "/api/v1/check_run"
	expectAPIKeyInQuery = true
	payloads = []*[]byte{}
	payloads = append(payloads, &expectedPayload)
	assert.Nil(t, forwarder.SubmitV1CheckRuns(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	<-wait
	<-wait
	require.Len(t, forwarder.retryQueue, 0)

	expectedPayload = []byte("SubmitV1Intake payload")
	expectedEndpoint = "/intake/"
	expectedHeaders.Set("Content-type", "application/json")
	expectAPIKeyInQuery = true
	payloads = []*[]byte{}
	payloads = append(payloads, &expectedPayload)
	assert.Nil(t, forwarder.SubmitV1Intake(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	<-wait
	<-wait
	require.Len(t, forwarder.retryQueue, 0)
}

func TestSubmitWithProxy(t *testing.T) {
	targetURL := "http://test:2121"
	firstKey := "api_key1"
	wait := make(chan bool)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { wait <- true }()
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, targetURL, fmt.Sprintf("%s://%s", r.URL.Scheme, r.URL.Host))
		assert.Equal(t, firstKey, r.Header.Get(apiHTTPHeaderKey))

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	config.Datadog.Set("proxy", map[string]interface{}{"http": ts.URL})
	defer config.Datadog.Set("proxy", nil)

	forwarder := NewDefaultForwarder(map[string][]string{targetURL: []string{firstKey}})
	// delay next retry cycle so we can inspect retry queue
	flushInterval = 5 * time.Hour
	forwarder.NumberOfWorkers = 1
	forwarder.Start()
	defer forwarder.Stop()

	payload := []byte("SubmitSeries payload")
	payloads := []*[]byte{}
	payloads = append(payloads, &payload)
	assert.Nil(t, forwarder.SubmitSeries(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	// wait for the queries to complete before changing expected value
	<-wait
	require.Len(t, forwarder.retryQueue, 0)
}

func TestSubmitWithProxyAndPassword(t *testing.T) {
	targetURL := "http://test:2121"
	userInfo := "testuser:password123456"
	expectedAuth := fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(userInfo)))
	firstKey := "api_key1"
	wait := make(chan bool)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { wait <- true }()
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, targetURL, fmt.Sprintf("%s://%s", r.URL.Scheme, r.URL.Host))
		assert.Equal(t, firstKey, r.Header.Get(apiHTTPHeaderKey))
		assert.Equal(t, expectedAuth, r.Header.Get("Proxy-Authorization"))

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	config.Datadog.Set("proxy", map[string]interface{}{"http": fmt.Sprintf("http://%s@%s", userInfo, ts.URL[7:])})
	defer config.Datadog.Set("proxy", nil)

	forwarder := NewDefaultForwarder(map[string][]string{targetURL: []string{firstKey}})
	// delay next retry cycle so we can inspect retry queue
	flushInterval = 5 * time.Hour
	forwarder.NumberOfWorkers = 1
	forwarder.Start()
	defer forwarder.Stop()

	payload := []byte("SubmitSeries payload")
	payloads := []*[]byte{}
	payloads = append(payloads, &payload)
	assert.Nil(t, forwarder.SubmitSeries(payloads, map[string]string{"Content-Type": "application/unit-test"}))
	// wait for the queries to complete before changing expected value
	<-wait
	require.Len(t, forwarder.retryQueue, 0)
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

	firstOut := <-forwarder.waitingPipe
	assert.Equal(t, firstOut, transaction2)

	secondOut := <-forwarder.waitingPipe
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
	require.Len(t, forwarder.waitingPipe, 0)
	// assert that the oldest transaction was dropped
	assert.Equal(t, transaction2, forwarder.retryQueue[0])
}
