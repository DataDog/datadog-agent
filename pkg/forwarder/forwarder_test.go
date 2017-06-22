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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
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
	assert.Equal(t, len(forwarder.workers), 0)
	assert.Equal(t, len(forwarder.retryQueue), 0)
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

	assert.Equal(t, len(forwarder.retryQueue), 0)
	assert.Equal(t, forwarder.internalState, Started)
	assert.NotNil(t, forwarder.waitingPipe)
	assert.NotNil(t, forwarder.requeuedTransaction)
	assert.NotNil(t, forwarder.stopRetry)

	assert.NotNil(t, forwarder.Start())
}

func TestSubmitInStopMode(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)

	assert.NotNil(t, forwarder)
	assert.NotNil(t, forwarder.SubmitTimeseries(nil))
	assert.NotNil(t, forwarder.SubmitEvent(nil))
	assert.NotNil(t, forwarder.SubmitCheckRun(nil))
	assert.NotNil(t, forwarder.SubmitHostMetadata(nil))
	assert.NotNil(t, forwarder.SubmitMetadata(nil))
}

func TestSubmit(t *testing.T) {
	expectedEndpoint := ""
	expectedQuery := ""
	expectedPayload := []byte{}
	expectedHeaders := make(http.Header)

	firstKey := "api_key1"
	secondKey := "api_key2"
	expectedHeaders.Set(apiHTTPHeaderKey, firstKey)

	wait := make(chan bool)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { wait <- true }()
		assert.Equal(t, r.Method, "POST")
		assert.Equal(t, r.URL.Path, expectedEndpoint)
		assert.Equal(t, r.URL.RawQuery, expectedQuery)
		for k := range expectedHeaders {
			assert.Equal(t, expectedHeaders.Get(k), r.Header.Get(k))
		}

		// We switch expected keys as the forwarder should create one
		// transaction per keys
		if expectedHeaders.Get(apiHTTPHeaderKey) == firstKey {
			expectedHeaders.Set(apiHTTPHeaderKey, secondKey)
		} else {
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

	payload := []byte("SubmitTimeseries payload")
	expectedPayload, _ = compression.Compress(nil, payload)
	expectedEndpoint = "/api/v2/series"
	assert.Nil(t, forwarder.SubmitTimeseries(&payload))
	// wait for the queries to complete before changing expected value
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	payload = []byte("SubmitEvent payload")
	expectedPayload, _ = compression.Compress(nil, payload)
	expectedEndpoint = "/api/v2/events"
	assert.Nil(t, forwarder.SubmitEvent(&payload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	payload = []byte("SubmitCheckRun payload")
	expectedPayload, _ = compression.Compress(nil, payload)
	expectedEndpoint = "/api/v2/check_runs"
	assert.Nil(t, forwarder.SubmitCheckRun(&payload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	payload = []byte("SubmitHostMetadata payload")
	expectedPayload, _ = compression.Compress(nil, payload)
	expectedEndpoint = "/api/v2/host_metadata"
	assert.Nil(t, forwarder.SubmitHostMetadata(&payload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	payload = []byte("SubmitMetadata payload")
	expectedPayload, _ = compression.Compress(nil, payload)
	expectedEndpoint = "/api/v2/metadata"
	assert.Nil(t, forwarder.SubmitMetadata(&payload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	expectedPayload = []byte("SubmitV1Series payload")
	expectedEndpoint = "/api/v1/series"
	expectedQuery = "api_key=test_api_key"
	assert.Nil(t, forwarder.SubmitV1Series("test_api_key", &expectedPayload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	expectedPayload = []byte("SubmitV1SketchSeries payload")
	expectedEndpoint = "/api/v1/sketches"
	expectedQuery = "api_key=test_api_key"
	assert.Nil(t, forwarder.SubmitV1SketchSeries("test_api_key", &expectedPayload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	expectedPayload = []byte("SubmitV1CheckRuns payload")
	expectedEndpoint = "/api/v1/check_run"
	expectedQuery = "api_key=test_api_key"
	assert.Nil(t, forwarder.SubmitV1CheckRuns("test_api_key", &expectedPayload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	expectedPayload = []byte("SubmitV1Intake payload")
	expectedEndpoint = "/intake/"
	expectedQuery = "api_key=test_api_key"
	expectedHeaders.Set("Content-type", "application/json")
	assert.Nil(t, forwarder.SubmitV1Intake("test_api_key", &expectedPayload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)
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
	config.Datadog.Set("proxy", ts.URL)
	defer config.Datadog.Set("proxy", nil)

	forwarder := NewDefaultForwarder(map[string][]string{targetURL: []string{firstKey}})
	// delay next retry cycle so we can inspect retry queue
	flushInterval = 5 * time.Hour
	forwarder.NumberOfWorkers = 1
	forwarder.Start()
	defer forwarder.Stop()

	payload := []byte("SubmitTimeseries payload")
	assert.Nil(t, forwarder.SubmitTimeseries(&payload))
	// wait for the queries to complete before changing expected value
	<-wait
	assert.Equal(t, 0, len(forwarder.retryQueue))
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
	config.Datadog.Set("proxy", fmt.Sprintf("http://%s@%s", userInfo, ts.URL[7:]))
	defer config.Datadog.Set("proxy", nil)

	forwarder := NewDefaultForwarder(map[string][]string{targetURL: []string{firstKey}})
	// delay next retry cycle so we can inspect retry queue
	flushInterval = 5 * time.Hour
	forwarder.NumberOfWorkers = 1
	forwarder.Start()
	defer forwarder.Stop()

	payload := []byte("SubmitTimeseries payload")
	assert.Nil(t, forwarder.SubmitTimeseries(&payload))
	// wait for the queries to complete before changing expected value
	<-wait
	assert.Equal(t, 0, len(forwarder.retryQueue))
}

func TestForwarderRetry(t *testing.T) {
	forwarder := NewDefaultForwarder(nil)
	forwarder.Start()
	defer forwarder.Stop()

	ready := newTestTransaction()
	notReady := newTestTransaction()

	forwarder.requeueTransaction(ready)
	forwarder.requeueTransaction(notReady)
	assert.Equal(t, len(forwarder.retryQueue), 2)

	ready.On("Process", forwarder.workers[0].Client).Return(nil).Times(1)
	ready.On("GetNextFlush").Return(time.Now()).Times(1)
	ready.On("GetCreatedAt").Return(time.Now()).Times(1)
	notReady.On("GetNextFlush").Return(time.Now().Add(10 * time.Minute)).Times(1)
	notReady.On("GetCreatedAt").Return(time.Now()).Times(1)

	forwarder.retryTransactions(time.Now())
	<-ready.processed

	ready.AssertExpectations(t)
	notReady.AssertExpectations(t)
	notReady.AssertNumberOfCalls(t, "Process", 0)
	assert.Equal(t, len(forwarder.retryQueue), 1)
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
	assert.Equal(t, len(forwarder.retryQueue), 0)
}
