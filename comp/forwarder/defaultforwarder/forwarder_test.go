// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	testDomain           = "http://app.datadoghq.com"
	testVersionDomain, _ = configUtils.AddAgentVersionToDomain(testDomain, "app")
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
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(keysPerDomains)))

	assert.NotNil(t, forwarder)
	assert.Equal(t, 1, forwarder.NumberOfWorkers)
	require.Len(t, forwarder.domainForwarders, 1) // only one domain has keys
	assert.Equal(t, resolver.NewSingleDomainResolvers(validKeysPerDomain), forwarder.domainResolvers)
	assert.Len(t, forwarder.domainForwarders, 1) // datadog.bar should have been dropped

	assert.Equal(t, forwarder.internalState.Load(), Stopped)
	assert.Equal(t, forwarder.State(), forwarder.internalState.Load())
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
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(monoKeysDomains)))
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
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(keysPerDomains)))
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
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(monoKeysDomains)))

	require.NotNil(t, forwarder)
	require.Equal(t, Stopped, forwarder.State())
	assert.NotNil(t, forwarder.SubmitSketchSeries(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitHostMetadata(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitMetadata(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitV1Series(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitSeries(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitV1Intake(nil, make(http.Header)))
	assert.NotNil(t, forwarder.SubmitV1CheckRuns(nil, make(http.Header)))
}

func TestCreateHTTPTransactions(t *testing.T) {
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(keysPerDomains)))
	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "foo"}
	p1 := []byte("A payload")
	p2 := []byte("Another payload")
	payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&p1, &p2})
	headers := make(http.Header)
	headers.Set("HTTP-MAGIC", "foo")

	transactions := forwarder.createHTTPTransactions(endpoint, payloads, headers)
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
	assert.Equal(t, p1, transactions[0].Payload.GetContent())
	assert.Equal(t, p1, transactions[1].Payload.GetContent())
	assert.Equal(t, p2, transactions[2].Payload.GetContent())
	assert.Equal(t, p2, transactions[3].Payload.GetContent())
}

func TestCreateHTTPTransactionsWithMultipleDomains(t *testing.T) {
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(keysWithMultipleDomains)))
	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "foo"}
	p1 := []byte("A payload")
	payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&p1})
	headers := make(http.Header)
	headers.Set("HTTP-MAGIC", "foo")

	transactions := forwarder.createHTTPTransactions(endpoint, payloads, headers)
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

	if txNormal[0].Headers.Get("DD-Api-Key") == "api-key-1" {
		assert.Equal(t, txNormal[0].Headers.Get("DD-Api-Key"), "api-key-1")
		assert.Equal(t, txNormal[1].Headers.Get("DD-Api-Key"), "api-key-2")
	} else {
		assert.Equal(t, txNormal[0].Headers.Get("DD-Api-Key"), "api-key-2")
		assert.Equal(t, txNormal[1].Headers.Get("DD-Api-Key"), "api-key-1")
	}

	assert.Equal(t, txBar[0].Headers.Get("DD-Api-Key"), "api-key-3")
}

func TestCreateHTTPTransactionsWithDifferentResolvers(t *testing.T) {
	resolvers := resolver.NewSingleDomainResolvers(keysWithMultipleDomains)
	additionalResolver := resolver.NewMultiDomainResolver("datadog.vector", []string{"api-key-4"})
	additionalResolver.RegisterAlternateDestination("diversion.domain", "diverted_name", resolver.Vector)
	resolvers["datadog.vector"] = additionalResolver
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolvers))
	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "diverted_name"}
	p1 := []byte("A payload")
	payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&p1})
	headers := make(http.Header)
	headers.Set("HTTP-MAGIC", "foo")

	transactions := forwarder.createHTTPTransactions(endpoint, payloads, headers)
	require.Len(t, transactions, 4, "should contain 4 transactions, contains %d", len(transactions))

	var txNormal, txBar, txVector []*transaction.HTTPTransaction
	for _, t := range transactions {
		if t.Domain == testVersionDomain {
			txNormal = append(txNormal, t)
		}
		if t.Domain == "datadog.bar" {
			txBar = append(txBar, t)
		}
		if t.Domain == "diversion.domain" {
			txVector = append(txVector, t)
		}
	}

	assert.Equal(t, len(txNormal), 2, "Two transactions should target the normal domain")
	assert.Equal(t, len(txBar), 1, "One transactions should target the normal domain")

	if txNormal[0].Headers.Get("DD-Api-Key") == "api-key-1" {
		assert.Equal(t, txNormal[0].Headers.Get("DD-Api-Key"), "api-key-1")
		assert.Equal(t, txNormal[1].Headers.Get("DD-Api-Key"), "api-key-2")
	} else {
		assert.Equal(t, txNormal[0].Headers.Get("DD-Api-Key"), "api-key-2")
		assert.Equal(t, txNormal[1].Headers.Get("DD-Api-Key"), "api-key-1")
	}
	assert.Equal(t, txBar[0].Headers.Get("DD-Api-Key"), "api-key-3")
	assert.Equal(t, txVector[0].Headers.Get("DD-Api-Key"), "api-key-4")
}

func TestCreateHTTPTransactionsWithOverrides(t *testing.T) {
	resolvers := make(map[string]resolver.DomainResolver)
	r := resolver.NewMultiDomainResolver(testDomain, []string{"api-key-1"})
	r.RegisterAlternateDestination("observability_pipelines_worker.tld", "diverted", resolver.Vector)
	resolvers[testDomain] = r
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolvers))

	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "no_diverted"}
	p1 := []byte("A payload")
	payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&p1})
	headers := make(http.Header)
	headers.Set("HTTP-MAGIC", "foo")

	transactions := forwarder.createHTTPTransactions(endpoint, payloads, headers)
	require.Len(t, transactions, 1, "should contain 1 transaction, contains %d", len(transactions))

	assert.Equal(t, transactions[0].Endpoint.Route, "/api/foo")
	assert.Equal(t, transactions[0].Domain, testVersionDomain)

	endpoint.Name = "diverted"
	transactions = forwarder.createHTTPTransactions(endpoint, payloads, headers)
	require.Len(t, transactions, 1, "should contain 1 transaction, contains %d", len(transactions))

	assert.Equal(t, transactions[0].Endpoint.Route, "/api/foo")
	assert.Equal(t, transactions[0].Domain, "observability_pipelines_worker.tld")
}

func TestArbitraryTagsHTTPHeader(t *testing.T) {
	mockConfig := config.Mock(t)
	mockConfig.Set("allow_arbitrary_tags", true)

	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(keysPerDomains)))
	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "foo"}
	payload := []byte("A payload")
	headers := make(http.Header)

	transactions := forwarder.createHTTPTransactions(endpoint, transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&payload}), headers)
	require.True(t, len(transactions) > 0)
	assert.Equal(t, "true", transactions[0].Headers.Get(arbitraryTagHTTPHeaderKey))
}

func TestSendHTTPTransactions(t *testing.T) {
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(keysPerDomains)))
	endpoint := transaction.Endpoint{Route: "/api/foo", Name: "foo"}
	p1 := []byte("A payload")
	payloads := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&p1})
	headers := make(http.Header)
	tr := forwarder.createHTTPTransactions(endpoint, payloads, headers)

	// fw is stopped, we should get an error
	err := forwarder.sendHTTPTransactions(tr)
	assert.NotNil(t, err)

	forwarder.Start()
	defer forwarder.Stop()
	err = forwarder.sendHTTPTransactions(tr)
	assert.Nil(t, err)
}

func TestSubmitV1Intake(t *testing.T) {
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	forwarder := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(monoKeysDomains)))
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
	assert.Nil(t, forwarder.SubmitV1Intake(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&p}), make(http.Header)))

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
	highPriorityQueueFull.Set(0)

	requests := atomic.NewInt64(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("%#v\n", r.URL)
		requests.Inc()
		w.WriteHeader(http.StatusOK)
	}))
	mockConfig := config.Mock(t)
	mockConfig.Set("dd_url", ts.URL)

	log := fxutil.Test[log.Component](t, log.MockModule)
	f := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(map[string][]string{ts.URL: {"api_key1", "api_key2"}, "invalid": {}, "invalid2": nil})))

	f.Start()
	defer f.Stop()

	data1 := []byte("data payload 1")
	data2 := []byte("data payload 2")
	payload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&data1, &data2})
	headers := http.Header{}
	headers.Set("key", "value")

	// - 2 requests to check the validity of the two api_key
	numReqs := int64(2)

	// for each call, we send 2 payloads * 2 api_keys
	assert.Nil(t, f.SubmitV1Series(payload, headers))
	numReqs += 4

	assert.Nil(t, f.SubmitSeries(payload, headers))
	numReqs += 4

	assert.Nil(t, f.SubmitV1Intake(payload, headers))
	numReqs += 4

	assert.Nil(t, f.SubmitV1CheckRuns(payload, headers))
	numReqs += 4

	assert.Nil(t, f.SubmitSketchSeries(payload, headers))
	numReqs += 4

	assert.Nil(t, f.SubmitHostMetadata(payload, headers))
	numReqs += 4

	assert.Nil(t, f.SubmitMetadata(payload, headers))
	numReqs += 4

	// let's wait a second for every channel communication to trigger
	<-time.After(1 * time.Second)

	// We should receive the following requests:
	// - 9 transactions * 2 payloads per transactions * 2 api_keys
	ts.Close()
	assert.Equal(t, numReqs, requests.Load())
}

func TestTransactionEventHandlers(t *testing.T) {
	requests := atomic.NewInt64(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Inc()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	mockConfig := config.Mock(t)
	mockConfig.Set("dd_url", ts.URL)

	log := fxutil.Test[log.Component](t, log.MockModule)
	f := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(map[string][]string{ts.URL: {"api_key1"}})))

	_ = f.Start()
	defer f.Stop()

	data := []byte("data payload 1")
	payload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&data})
	headers := http.Header{}
	headers.Set("key", "value")

	transactions := f.createHTTPTransactions(endpoints.SeriesEndpoint, payload, headers)
	require.Len(t, transactions, 1)

	attempts := atomic.NewInt64(0)

	var wg sync.WaitGroup
	wg.Add(1)
	transactions[0].CompletionHandler = func(transaction *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
		assert.Equal(t, http.StatusOK, statusCode)
		wg.Done()
	}
	transactions[0].AttemptHandler = func(transaction *transaction.HTTPTransaction) {
		attempts.Inc()
	}

	err := f.sendHTTPTransactions(transactions)
	require.NoError(t, err)

	wg.Wait()

	assert.Equal(t, int64(1), attempts.Load())
}

func TestTransactionEventHandlersOnRetry(t *testing.T) {
	requests := atomic.NewInt64(0)

	mux := http.NewServeMux()
	mux.HandleFunc(endpoints.V1ValidateEndpoint.Route, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(endpoints.SeriesEndpoint.Route, func(w http.ResponseWriter, r *http.Request) {
		if v := requests.Inc(); v == 1 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	mockConfig := config.Mock(t)
	mockConfig.Set("dd_url", ts.URL)

	log := fxutil.Test[log.Component](t, log.MockModule)
	f := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(map[string][]string{ts.URL: {"api_key1"}})))

	_ = f.Start()
	defer f.Stop()

	data := []byte("data payload 1")
	payload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&data})
	headers := http.Header{}
	headers.Set("key", "value")

	transactions := f.createHTTPTransactions(endpoints.SeriesEndpoint, payload, headers)
	require.Len(t, transactions, 1)

	attempts := atomic.NewInt64(0)

	var wg sync.WaitGroup
	wg.Add(1)
	transactions[0].CompletionHandler = func(transaction *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
		assert.Equal(t, http.StatusOK, statusCode)
		wg.Done()
	}
	transactions[0].AttemptHandler = func(transaction *transaction.HTTPTransaction) {
		attempts.Inc()
	}

	err := f.sendHTTPTransactions(transactions)
	require.NoError(t, err)

	wg.Wait()

	assert.Equal(t, int64(2), attempts.Load())
}

func TestTransactionEventHandlersNotRetryable(t *testing.T) {
	requests := atomic.NewInt64(0)

	mux := http.NewServeMux()
	mux.HandleFunc(endpoints.V1ValidateEndpoint.Route, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(endpoints.SeriesEndpoint.Route, func(w http.ResponseWriter, r *http.Request) {
		requests.Inc()
		w.WriteHeader(http.StatusInternalServerError)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	mockConfig := config.Mock(t)
	mockConfig.Set("dd_url", ts.URL)

	log := fxutil.Test[log.Component](t, log.MockModule)
	f := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(map[string][]string{ts.URL: {"api_key1"}})))

	_ = f.Start()
	defer f.Stop()

	data := []byte("data payload 1")
	payload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&data})
	headers := http.Header{}
	headers.Set("key", "value")

	transactions := f.createHTTPTransactions(endpoints.SeriesEndpoint, payload, headers)
	require.Len(t, transactions, 1)

	attempts := atomic.NewInt64(0)

	var wg sync.WaitGroup
	wg.Add(1)
	transactions[0].CompletionHandler = func(transaction *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
		assert.Equal(t, http.StatusInternalServerError, statusCode)
		wg.Done()
	}
	transactions[0].AttemptHandler = func(transaction *transaction.HTTPTransaction) {
		attempts.Inc()
	}

	transactions[0].Retryable = false

	err := f.sendHTTPTransactions(transactions)
	require.NoError(t, err)

	wg.Wait()

	assert.Equal(t, int64(1), requests.Load())
	assert.Equal(t, int64(1), attempts.Load())
}

func TestProcessLikePayloadResponseTimeout(t *testing.T) {
	requests := atomic.NewInt64(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Inc()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	mockConfig := config.Mock(t)
	responseTimeout := defaultResponseTimeout

	defaultResponseTimeout = 5 * time.Second
	mockConfig.Set("dd_url", ts.URL)
	mockConfig.Set("forwarder_num_workers", 0) // Set the number of workers to 0 so the txn goes nowhere
	defer func() {
		defaultResponseTimeout = responseTimeout
	}()

	log := fxutil.Test[log.Component](t, log.MockModule)
	f := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(map[string][]string{ts.URL: {"api_key1"}})))

	_ = f.Start()
	defer f.Stop()

	data := []byte("data payload 1")
	payload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&data})
	headers := http.Header{}
	headers.Set("key", "value")

	transactions := f.createHTTPTransactions(endpoints.SeriesEndpoint, payload, headers)
	require.Len(t, transactions, 1)

	responses, err := f.submitProcessLikePayload(endpoints.SeriesEndpoint, payload, headers, true)
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
		body, err := io.ReadAll(r.Body)
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

	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	f := NewDefaultForwarder(mockConfig, log, NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(map[string][]string{ts.URL: {"api_key1"}})))

	f.Start()
	defer f.Stop()

	data1 := []byte("data payload 1")
	data2 := []byte("data payload 2")
	dataHighPrio := []byte("data payload high Prio")
	headers := http.Header{}
	headers.Set("key", "value")

	assert.Nil(t, f.SubmitMetadata(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&data1}), headers))
	// Wait so that GetCreatedAt returns a different value for each HTTPTransaction
	time.Sleep(10 * time.Millisecond)

	// SubmitHostMetadata send the transactions as TransactionPriorityHigh
	assert.Nil(t, f.SubmitHostMetadata(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&dataHighPrio}), headers))
	time.Sleep(10 * time.Millisecond)
	assert.Nil(t, f.SubmitMetadata(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&data2}), headers))

	assert.Equal(t, string(dataHighPrio), <-requestChan)
	assert.Equal(t, string(data2), <-requestChan)
	assert.Equal(t, string(data1), <-requestChan)
}

func TestCustomCompletionHandler(t *testing.T) {
	highPriorityQueueFull.Set(0)

	// Setup a test HTTP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Point agent configuration to it
	cfg := config.Mock(t)
	cfg.Set("dd_url", srv.URL)

	// Now let's create a Forwarder with a custom HTTPCompletionHandler set to it
	done := make(chan struct{})
	defer close(done)
	var handler transaction.HTTPCompletionHandler = func(transaction *transaction.HTTPTransaction, statusCode int, body []byte, err error) {
		done <- struct{}{}
	}
	mockConfig := config.Mock(t)
	log := fxutil.Test[log.Component](t, log.MockModule)
	options := NewOptionsWithResolvers(mockConfig, log, resolver.NewSingleDomainResolvers(map[string][]string{
		srv.URL: {"api_key1"},
	}))
	options.CompletionHandler = handler

	f := NewDefaultForwarder(mockConfig, log, options)
	f.Start()
	defer f.Stop()

	data := []byte("payload_data")
	payload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&data})
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
