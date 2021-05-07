// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	testDomain           = "http://app.datadoghq.com"
	testVersionDomain, _ = config.AddAgentVersionToDomain(testDomain, "app")
	monoKeysDomains      = map[string][]string{
		testVersionDomain: {"monokey"},
	}
	keysPerDomains = map[string][]string{
		testDomain:    {"api-key-1", "api-key-2"},
		"datadog.bar": nil,
	}
	keysWithMultipleDomains = map[string][]string{
		testDomain:    {"api-key-1", "api-key-2"},
		"datadog.bar": {"api-key-3"},
	}
	validKeysPerDomain = map[string][]string{
		testVersionDomain: {"api-key-1", "api-key-2"},
	}
)

func TestNewDefaultForwarder(t *testing.T) {
	forwarder := NewDefaultForwarder(NewOptions(keysPerDomains))

	assert.NotNil(t, forwarder)
	assert.Equal(t, 1, forwarder.NumberOfWorkers)
	require.Len(t, forwarder.domainForwarders, 1) // only one domain has keys
	assert.Equal(t, validKeysPerDomain, forwarder.keysPerDomains)
	assert.Len(t, forwarder.domainForwarders, 1) // datadog.bar should have been dropped

	assert.Equal(t, forwarder.internalState, Stopped)
	assert.Equal(t, forwarder.State(), forwarder.internalState)
}

func TestFeature(t *testing.T) {
	var featureSet Features

	featureSet = SetFeature(featureSet, CoreFeatures)
	featureSet = SetFeature(featureSet, ProcessFeatures)
	assert.True(t, HasFeature(featureSet, CoreFeatures))
	assert.True(t, HasFeature(featureSet, ProcessFeatures))

	featureSet = ClearFeature(featureSet, CoreFeatures)
	assert.False(t, HasFeature(featureSet, CoreFeatures))
	assert.True(t, HasFeature(featureSet, ProcessFeatures))

	featureSet = ToggleFeature(featureSet, ProcessFeatures)
	assert.False(t, HasFeature(featureSet, CoreFeatures))
	assert.False(t, HasFeature(featureSet, ProcessFeatures))
}

func TestStart(t *testing.T) {
	forwarder := NewDefaultForwarder(NewOptions(monoKeysDomains))
	err := forwarder.Start()
	defer forwarder.Stop()

	assert.Nil(t, err)
	assert.Equal(t, Started, forwarder.State())
	require.Len(t, forwarder.domainForwarders, 1)
	require.NotNil(t, forwarder.healthChecker)
	assert.NotNil(t, forwarder.Start())
}

func TestStopWithoutPurgingTransaction(t *testing.T) {
	forwarderTimeout := config.Datadog.GetDuration("forwarder_stop_timeout")
	defer func() { config.Datadog.Set("forwarder_stop_timeout", forwarderTimeout) }()
	config.Datadog.Set("forwarder_stop_timeout", 0)

	testStop(t)
}

func TestStopWithPurgingTransaction(t *testing.T) {
	forwarderTimeout := config.Datadog.GetDuration("forwarder_stop_timeout")
	defer func() { config.Datadog.Set("forwarder_stop_timeout", forwarderTimeout) }()
	config.Datadog.Set("forwarder_stop_timeout", 1)

	testStop(t)
}

func testStop(t *testing.T) {
	forwarder := NewDefaultForwarder(NewOptions(keysPerDomains))
	assert.Equal(t, Stopped, forwarder.State())
	forwarder.Stop() // this should be a noop
	forwarder.Start()
	domainForwarders := forwarder.domainForwarders
	forwarder.Stop()
	assert.Equal(t, Stopped, forwarder.State())
	assert.Nil(t, forwarder.healthChecker)
	assert.Len(t, forwarder.domainForwarders, 0)
	for _, df := range domainForwarders {
		assert.Equal(t, Stopped, df.internalState)
	}
}

func TestSubmitIfStopped(t *testing.T) {
	forwarder := NewDefaultForwarder(NewOptions(monoKeysDomains))

	require.NotNil(t, forwarder)
	require.Equal(t, Stopped, forwarder.State())
	assert.NotNil(t, forwarder.SubmitSeries(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitEvents(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitServiceChecks(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitSketchSeries(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitHostMetadata(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitMetadata(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitV1Series(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitV1Intake(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitV1CheckRuns(nil, make(http.Header)))
}

func TestCreateHTTPTransactions(t *testing.T) {
	forwarder := NewDefaultForwarder(NewOptions(keysPerDomains))
	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "foo"}
	p1 := []byte("A payload")
	p2 := []byte("Another payload")
	payloads := Payloads{&p1, &p2}
	headers := make(http.Header)
	headers.Set("HTTP-MAGIC", "foo")

	transactions := forwarder.createHTTPTransactions(endpoint, payloads, false, headers)
	require.Len(t, transactions, 4)
	assert.Equal(t, testVersionDomain, transactions[0].Domain)
	assert.Equal(t, testVersionDomain, transactions[1].Domain)
	assert.Equal(t, testVersionDomain, transactions[2].Domain)
	assert.Equal(t, testVersionDomain, transactions[3].Domain)
	assert.Equal(t, endpoint.Route, transactions[0].Endpoint.Route)
	assert.Equal(t, endpoint.Route, transactions[1].Endpoint.Route)
	assert.Equal(t, endpoint.Route, transactions[2].Endpoint.Route)
	assert.Equal(t, endpoint.Route, transactions[3].Endpoint.Route)
	assert.Len(t, transactions[0].Headers, 4)
	assert.NotEmpty(t, transactions[0].Headers.Get("DD-Api-Key"))
	assert.NotEmpty(t, transactions[0].Headers.Get("HTTP-MAGIC"))
	assert.Equal(t, version.AgentVersion, transactions[0].Headers.Get("DD-Agent-Version"))
	assert.Equal(t, "datadog-agent/"+version.AgentVersion, transactions[0].Headers.Get("User-Agent"))
	assert.Equal(t, "", transactions[0].Headers.Get(arbitraryTagHTTPHeaderKey))
	assert.Equal(t, p1, *(transactions[0].Payload))
	assert.Equal(t, p1, *(transactions[1].Payload))
	assert.Equal(t, p2, *(transactions[2].Payload))
	assert.Equal(t, p2, *(transactions[3].Payload))

	transactions = forwarder.createHTTPTransactions(endpoint, payloads, true, headers)
	require.Len(t, transactions, 4)
	assert.Contains(t, transactions[0].Endpoint.Route, "api_key=api-key-1")
	assert.Contains(t, transactions[1].Endpoint.Route, "api_key=api-key-2")
	assert.Contains(t, transactions[2].Endpoint.Route, "api_key=api-key-1")
	assert.Contains(t, transactions[3].Endpoint.Route, "api_key=api-key-2")
}

func TestCreateHTTPTransactionsWithMultipleDomains(t *testing.T) {
	forwarder := NewDefaultForwarder(NewOptions(keysWithMultipleDomains))
	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "foo"}
	p1 := []byte("A payload")
	payloads := Payloads{&p1}
	headers := make(http.Header)
	headers.Set("HTTP-MAGIC", "foo")

	transactions := forwarder.createHTTPTransactions(endpoint, payloads, true, headers)
	require.Len(t, transactions, 3, "should contain 3 transactions, contains %d", len(transactions))

	var txNormal, txBar []*transaction.HTTPTransaction
	for _, t := range transactions {
		if t.Domain == testVersionDomain {
			txNormal = append(txNormal, t)
		}
		if t.Domain == "datadog.bar" {
			txBar = append(txBar, t)
		}
	}

	assert.Equal(t, len(txNormal), 2, "Two transactions should target the normal domain")
	assert.Equal(t, len(txBar), 1, "One transactions should target the normal domain")

	if strings.HasSuffix(txNormal[0].Endpoint.Route, "api-key-1") {
		assert.Equal(t, txNormal[0].Endpoint.Route, "/api/foo?api_key=api-key-1")
		assert.Equal(t, txNormal[1].Endpoint.Route, "/api/foo?api_key=api-key-2")
	} else {
		assert.Equal(t, txNormal[0].Endpoint.Route, "/api/foo?api_key=api-key-2")
		assert.Equal(t, txNormal[1].Endpoint.Route, "/api/foo?api_key=api-key-1")
	}
	assert.Equal(t, txBar[0].Endpoint.Route, "/api/foo?api_key=api-key-3")
}

func TestArbitraryTagsHTTPHeader(t *testing.T) {
	mockConfig := config.Mock()
	mockConfig.Set("allow_arbitrary_tags", true)
	defer mockConfig.Set("allow_arbitrary_tags", false)

	forwarder := NewDefaultForwarder(NewOptions(keysPerDomains))
	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "foo"}
	payload := []byte("A payload")
	headers := make(http.Header)

	transactions := forwarder.createHTTPTransactions(endpoint, Payloads{&payload}, false, headers)
	require.True(t, len(transactions) > 0)
	assert.Equal(t, "true", transactions[0].Headers.Get(arbitraryTagHTTPHeaderKey))
}

func TestSendHTTPTransactions(t *testing.T) {
	forwarder := NewDefaultForwarder(NewOptions(keysPerDomains))
	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "foo"}
	p1 := []byte("A payload")
	payloads := Payloads{&p1}
	headers := make(http.Header)
	tr := forwarder.createHTTPTransactions(endpoint, payloads, false, headers)

	// fw is stopped, we should get an error
	err := forwarder.sendHTTPTransactions(tr)
	assert.NotNil(t, err)

	forwarder.Start()
	defer forwarder.Stop()
	err = forwarder.sendHTTPTransactions(tr)
	assert.Nil(t, err)
}

func TestSubmitV1Intake(t *testing.T) {
	forwarder := NewDefaultForwarder(NewOptions(monoKeysDomains))
	forwarder.Start()
	defer forwarder.Stop()

	// Overwrite domainForwarders input channel. We are testing that the
	// DefaultForwarder correctly create HTTPTransaction, set the headers
	// and send them to the correct domainForwarder.
	inputQueue := make(chan transaction.Transaction, 1)
	df := forwarder.domainForwarders[testVersionDomain]
	bk := df.highPrio
	df.highPrio = inputQueue
	defer func() { df.highPrio = bk }()

	p := []byte("test")
	assert.Nil(t, forwarder.SubmitV1Intake(Payloads{&p}, make(http.Header)))

	select {
	case tr := <-df.highPrio:
		require.NotNil(t, tr)
		httpTr := tr.(*transaction.HTTPTransaction)
		assert.Equal(t, "application/json", httpTr.Headers.Get("Content-Type"))
	case <-time.After(1 * time.Second):
		require.Fail(t, "highPrio queue should contain a transaction")
	}
}

// TestForwarderEndtoEnd is a simple test to see if a payload is well broadcast
// between every components of the forwarder. Corner cases and error are tested
// per component.
func TestForwarderEndtoEnd(t *testing.T) {
	// reseting DroppedOnInput
	transactionsDroppedOnInput.Set(0)

	requests := int64(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	mockConfig := config.Mock()
	ddURL := mockConfig.Get("dd_url")
	mockConfig.Set("dd_url", ts.URL)
	defer mockConfig.Set("dd_url", ddURL)

	f := NewDefaultForwarder(NewOptions(map[string][]string{
		ts.URL:     {"api_key1", "api_key2"},
		"invalid":  {},
		"invalid2": nil,
	}))

	f.Start()
	defer f.Stop()

	data1 := []byte("data payload 1")
	data2 := []byte("data payload 2")
	payload := Payloads{&data1, &data2}
	headers := http.Header{}
	headers.Set("key", "value")

	assert.Nil(t, f.SubmitV1Series(payload, headers))
	assert.Nil(t, f.SubmitV1Intake(payload, headers))
	assert.Nil(t, f.SubmitV1CheckRuns(payload, headers))
	assert.Nil(t, f.SubmitSeries(payload, headers))
	assert.Nil(t, f.SubmitEvents(payload, headers))
	assert.Nil(t, f.SubmitServiceChecks(payload, headers))
	assert.Nil(t, f.SubmitSketchSeries(payload, headers))
	assert.Nil(t, f.SubmitHostMetadata(payload, headers))
	assert.Nil(t, f.SubmitMetadata(payload, headers))

	// let's wait a second for every channel communication to trigger
	<-time.After(1 * time.Second)

	// We should receive 38 requests:
	// - 9 transactions * 2 payloads per transactions * 2 api_keys
	// - 2 requests to check the validity of the two api_key
	ts.Close()
	assert.Equal(t, int64(38), requests)
}

func TestTransactionEventHandlers(t *testing.T) {
	requests := int64(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	mockConfig := config.Mock()
	ddURL := mockConfig.Get("dd_url")
	mockConfig.Set("dd_url", ts.URL)
	defer mockConfig.Set("dd_url", ddURL)

	f := NewDefaultForwarder(NewOptions(map[string][]string{
		ts.URL: {"api_key1"},
	}))

	_ = f.Start()
	defer f.Stop()

	data := []byte("data payload 1")
	payload := Payloads{&data}
	headers := http.Header{}
	headers.Set("key", "value")

	transactions := f.createHTTPTransactions(metadataEndpoint, payload, false, headers)
	require.Len(t, transactions, 1)

	attempts := int64(0)

	var wg sync.WaitGroup
	wg.Add(1)
	transactions[0].CompletionHandler = func(transaction *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
		assert.Equal(t, http.StatusOK, statusCode)
		wg.Done()
	}
	transactions[0].AttemptHandler = func(transaction *transaction.HTTPTransaction) {
		atomic.AddInt64(&attempts, 1)
	}

	err := f.sendHTTPTransactions(transactions)
	require.NoError(t, err)

	wg.Wait()

	assert.Equal(t, int64(1), atomic.LoadInt64(&attempts))
}

func TestTransactionEventHandlersOnRetry(t *testing.T) {
	requests := int64(0)

	mux := http.NewServeMux()
	mux.HandleFunc(v1ValidateEndpoint.Route, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(metadataEndpoint.Route, func(w http.ResponseWriter, r *http.Request) {
		if v := atomic.AddInt64(&requests, 1); v == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	mockConfig := config.Mock()
	ddURL := mockConfig.Get("dd_url")
	mockConfig.Set("dd_url", ts.URL)
	defer mockConfig.Set("dd_url", ddURL)

	f := NewDefaultForwarder(NewOptions(map[string][]string{
		ts.URL: {"api_key1"},
	}))

	_ = f.Start()
	defer f.Stop()

	data := []byte("data payload 1")
	payload := Payloads{&data}
	headers := http.Header{}
	headers.Set("key", "value")

	transactions := f.createHTTPTransactions(metadataEndpoint, payload, false, headers)
	require.Len(t, transactions, 1)

	attempts := int64(0)

	var wg sync.WaitGroup
	wg.Add(1)
	transactions[0].CompletionHandler = func(transaction *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
		assert.Equal(t, http.StatusOK, statusCode)
		wg.Done()
	}
	transactions[0].AttemptHandler = func(transaction *transaction.HTTPTransaction) {
		atomic.AddInt64(&attempts, 1)
	}

	err := f.sendHTTPTransactions(transactions)
	require.NoError(t, err)

	wg.Wait()

	assert.Equal(t, int64(2), atomic.LoadInt64(&attempts))
}

func TestTransactionEventHandlersNotRetryable(t *testing.T) {
	requests := int64(0)

	mux := http.NewServeMux()
	mux.HandleFunc(v1ValidateEndpoint.Route, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(metadataEndpoint.Route, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requests, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	mockConfig := config.Mock()
	ddURL := mockConfig.Get("dd_url")
	mockConfig.Set("dd_url", ts.URL)
	defer mockConfig.Set("dd_url", ddURL)

	f := NewDefaultForwarder(NewOptions(map[string][]string{
		ts.URL: {"api_key1"},
	}))

	_ = f.Start()
	defer f.Stop()

	data := []byte("data payload 1")
	payload := Payloads{&data}
	headers := http.Header{}
	headers.Set("key", "value")

	transactions := f.createHTTPTransactions(metadataEndpoint, payload, false, headers)
	require.Len(t, transactions, 1)

	attempts := int64(0)

	var wg sync.WaitGroup
	wg.Add(1)
	transactions[0].CompletionHandler = func(transaction *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
		assert.Equal(t, http.StatusInternalServerError, statusCode)
		wg.Done()
	}
	transactions[0].AttemptHandler = func(transaction *transaction.HTTPTransaction) {
		atomic.AddInt64(&attempts, 1)
	}

	transactions[0].Retryable = false

	err := f.sendHTTPTransactions(transactions)
	require.NoError(t, err)

	wg.Wait()

	assert.Equal(t, int64(1), atomic.LoadInt64(&requests))
	assert.Equal(t, int64(1), atomic.LoadInt64(&attempts))
}

func TestProcessLikePayloadResponseTimeout(t *testing.T) {
	requests := int64(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requests, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	mockConfig := config.Mock()
	ddURL := mockConfig.Get("dd_url")
	numWorkers := mockConfig.Get("forwarder_num_workers")
	responseTimeout := defaultResponseTimeout

	defaultResponseTimeout = 5 * time.Second
	mockConfig.Set("dd_url", ts.URL)
	mockConfig.Set("forwarder_num_workers", 0) // Set the number of workers to 0 so the txn goes nowhere
	defer func() {
		mockConfig.Set("dd_url", ddURL)
		mockConfig.Set("forwarder_num_workers", numWorkers)
		defaultResponseTimeout = responseTimeout
	}()

	f := NewDefaultForwarder(NewOptions(map[string][]string{
		ts.URL: {"api_key1"},
	}))

	_ = f.Start()
	defer f.Stop()

	data := []byte("data payload 1")
	payload := Payloads{&data}
	headers := http.Header{}
	headers.Set("key", "value")

	transactions := f.createHTTPTransactions(metadataEndpoint, payload, false, headers)
	require.Len(t, transactions, 1)

	responses, err := f.submitProcessLikePayload(metadataEndpoint, payload, headers, true)
	require.NoError(t, err)

	_, ok := <-responses
	require.False(t, ok) // channel should have been closed without receiving any responses
}

func TestHighPriorityTransaction(t *testing.T) {
	var receivedRequests = make(map[string]struct{})
	var mutex sync.Mutex
	var requestChan = make(chan (string))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		defer r.Body.Close()
		body, err := ioutil.ReadAll(r.Body)
		assert.NoError(t, err)
		bodyStr := string(body)

		// Failed the first time for each request
		if _, found := receivedRequests[bodyStr]; !found {
			receivedRequests[bodyStr] = struct{}{}
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
			requestChan <- bodyStr
		}
	}))

	config.Datadog.Set("forwarder_backoff_max", 0.5)
	defer config.Datadog.Set("forwarder_backoff_max", nil)

	oldFlushInterval := flushInterval
	flushInterval = 500 * time.Millisecond
	defer func() { flushInterval = oldFlushInterval }()

	f := NewDefaultForwarder(NewOptions(map[string][]string{
		ts.URL: {"api_key1"},
	}))

	f.Start()
	defer f.Stop()

	data1 := []byte("data payload 1")
	data2 := []byte("data payload 2")
	dataHighPrio := []byte("data payload high Prio")
	headers := http.Header{}
	headers.Set("key", "value")

	assert.Nil(t, f.SubmitMetadata(Payloads{&data1}, headers))
	// Wait so that GetCreatedAt returns a different value for each HTTPTransaction
	time.Sleep(10 * time.Millisecond)

	// SubmitHostMetadata send the transactions as TransactionPriorityHigh
	assert.Nil(t, f.SubmitHostMetadata(Payloads{&dataHighPrio}, headers))
	time.Sleep(10 * time.Millisecond)
	assert.Nil(t, f.SubmitMetadata(Payloads{&data2}, headers))

	assert.Equal(t, string(dataHighPrio), <-requestChan)
	assert.Equal(t, string(data2), <-requestChan)
	assert.Equal(t, string(data1), <-requestChan)
}

func TestCustomCompletionHandler(t *testing.T) {
	transactionsDroppedOnInput.Set(0)

	// Setup a test HTTP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Point agent configuration to it
	cfg := config.Mock()
	prevURL := cfg.Get("dd_url")
	defer cfg.Set("dd_url", prevURL)
	cfg.Set("dd_url", srv.URL)

	// Now let's create a Forwarder with a custom HTTPCompletionHandler set to it
	done := make(chan struct{})
	defer close(done)
	var handler transaction.HTTPCompletionHandler = func(transaction *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
		done <- struct{}{}
	}

	options := NewOptions(map[string][]string{
		srv.URL: {"api_key1"},
	})
	options.CompletionHandler = handler

	f := NewDefaultForwarder(options)
	f.Start()
	defer f.Stop()

	data := []byte("payload_data")
	payload := Payloads{&data}
	assert.Nil(t, f.SubmitV1Series(payload, http.Header{}))

	// And finally let's ensure the handler gets called
	var handlerCalled bool
	select {
	case <-done:
		handlerCalled = true
	case <-time.After(time.Second):
	}

	assert.True(t, handlerCalled)
}
