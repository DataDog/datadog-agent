// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package forwarder

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
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
	validKeysPerDomain = map[string][]string{
		testVersionDomain: {"api-key-1", "api-key-2"},
	}
)

func TestNewDefaultForwarder(t *testing.T) {
	forwarder := NewDefaultForwarder(keysPerDomains)

	assert.NotNil(t, forwarder)
	assert.Equal(t, 1, forwarder.NumberOfWorkers)
	require.Len(t, forwarder.domainForwarders, 1) // only one domain has keys
	assert.Equal(t, validKeysPerDomain, forwarder.keysPerDomains)
	assert.Len(t, forwarder.domainForwarders, 1) // datadog.bar should have been dropped

	assert.Equal(t, forwarder.internalState, Stopped)
	assert.Equal(t, forwarder.State(), forwarder.internalState)
}

func TestStart(t *testing.T) {
	forwarder := NewDefaultForwarder(monoKeysDomains)
	err := forwarder.Start()
	defer forwarder.Stop()

	assert.Nil(t, err)
	assert.Equal(t, Started, forwarder.State())
	require.Len(t, forwarder.domainForwarders, 1)
	require.NotNil(t, forwarder.healthChecker)
	assert.NotNil(t, forwarder.Start())
}

func TestStop(t *testing.T) {
	forwarder := NewDefaultForwarder(keysPerDomains)
	assert.Equal(t, Stopped, forwarder.State())
	forwarder.Stop() // this should be a noop
	forwarder.Start()
	forwarder.Stop()
	assert.Equal(t, Stopped, forwarder.State())
	assert.Nil(t, forwarder.healthChecker)
	assert.Len(t, forwarder.domainForwarders, 0)
}

func TestSubmitIfStopped(t *testing.T) {
	forwarder := NewDefaultForwarder(monoKeysDomains)

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
	forwarder := NewDefaultForwarder(keysPerDomains)
	endpoint := "/api/foo"
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
	defer forwarder.Stop()
	err = forwarder.sendHTTPTransactions(tr)
	assert.Nil(t, err)
}

func TestSubmitV1Intake(t *testing.T) {
	forwarder := NewDefaultForwarder(monoKeysDomains)
	forwarder.Start()
	defer forwarder.Stop()

	// Overwrite domainForwarders input channel. We are testing that the
	// DefaultForwarder correctly create HTTPTransaction, set the headers
	// and send them to the correct domainForwarder.
	inputQueue := make(chan Transaction, 1)
	df := forwarder.domainForwarders[testVersionDomain]
	bk := df.highPrio
	df.highPrio = inputQueue
	defer func() { df.highPrio = bk }()

	p := []byte("test")
	assert.Nil(t, forwarder.SubmitV1Intake(Payloads{&p}, make(http.Header)))

	select {
	case tr := <-df.highPrio:
		require.NotNil(t, tr)
		httpTr := tr.(*HTTPTransaction)
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
	mockConfig := config.NewMock()
	ddURL := mockConfig.Get("dd_url")
	mockConfig.Set("dd_url", ts.URL)
	defer mockConfig.Set("dd_url", ddURL)

	f := NewDefaultForwarder(map[string][]string{
		ts.URL:     {"api_key1", "api_key2"},
		"invalid":  {},
		"invalid2": nil,
	})

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
