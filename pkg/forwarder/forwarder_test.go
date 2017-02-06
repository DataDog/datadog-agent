package forwarder

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewForwarder(t *testing.T) {
	keysPerDomains := map[string][]string{"domainsA": []string{"key1", "key2"}, "domainsB": nil}
	forwarder := NewForwarder(keysPerDomains)

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
	forwarder := NewForwarder(nil)

	assert.NotNil(t, forwarder)
	assert.Equal(t, forwarder.State(), Stopped)
	forwarder.internalState = Started
	assert.Equal(t, forwarder.State(), Started)
}

func TestForwarderStart(t *testing.T) {
	forwarder := NewForwarder(nil)

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
	forwarder := NewForwarder(nil)

	assert.NotNil(t, forwarder)
	assert.NotNil(t, forwarder.SubmitTimeseries(nil))
	assert.NotNil(t, forwarder.SubmitEvent(nil))
	assert.NotNil(t, forwarder.SubmitCheckRun(nil))
	assert.NotNil(t, forwarder.SubmitHostMetadata(nil))
	assert.NotNil(t, forwarder.SubmitMetadata(nil))
}

func TestSubmit(t *testing.T) {
	expectedEndpoint := ""
	expectedPayload := []byte{}

	firstKey := "api_key1"
	secondKey := "api_key2"
	expectedAPIKey := firstKey

	wait := make(chan bool)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { wait <- true }()
		assert.Equal(t, r.Method, "POST")
		assert.Equal(t, r.URL.Path, expectedEndpoint)
		assert.Equal(t, r.Header.Get(apiHTTPHeaderKey), expectedAPIKey)

		// We switch expected keys as the forwarder should create one
		// transaction per keys
		if expectedAPIKey == firstKey {
			expectedAPIKey = secondKey
		} else {
			expectedAPIKey = firstKey
		}

		body, err := ioutil.ReadAll(r.Body)
		assert.Nil(t, err)
		assert.Equal(t, body, expectedPayload)

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	forwarder := NewForwarder(map[string][]string{ts.URL: []string{firstKey, secondKey}})
	// delay next retry cycle so we can inspect retry queue
	flushInterval = 5 * time.Hour
	forwarder.NumberOfWorkers = 1
	forwarder.Start()
	defer forwarder.Stop()

	expectedPayload = []byte("SubmitTimeseries payload")
	expectedEndpoint = "/api/v2/series"
	assert.Nil(t, forwarder.SubmitTimeseries(&expectedPayload))
	// wait for the queries to complete before changing expected value
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	expectedPayload = []byte("SubmitEvent payload")
	expectedEndpoint = "/api/v2/events"
	assert.Nil(t, forwarder.SubmitEvent(&expectedPayload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	expectedPayload = []byte("SubmitCheckRun payload")
	expectedEndpoint = "/api/v2/check_runs"
	assert.Nil(t, forwarder.SubmitCheckRun(&expectedPayload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	expectedPayload = []byte("SubmitHostMetadata payload")
	expectedEndpoint = "/api/v2/host_metadata"
	assert.Nil(t, forwarder.SubmitHostMetadata(&expectedPayload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)

	expectedPayload = []byte("SubmitMetadata payload")
	expectedEndpoint = "/api/v2/metadata"
	assert.Nil(t, forwarder.SubmitMetadata(&expectedPayload))
	<-wait
	<-wait
	assert.Equal(t, len(forwarder.retryQueue), 0)
}

func TestForwarderRetry(t *testing.T) {
	forwarder := NewForwarder(nil)
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
	forwarder := NewForwarder(nil)
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
