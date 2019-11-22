// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package forwarder

import (
	"expvar"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	forwarderExpvars          = expvar.NewMap("forwarder")
	transactionsExpvars       = expvar.Map{}
	transactionsSeries        = expvar.Int{}
	transactionsEvents        = expvar.Int{}
	transactionsServiceChecks = expvar.Int{}
	transactionsSketchSeries  = expvar.Int{}
	transactionsHostMetadata  = expvar.Int{}
	transactionsMetadata      = expvar.Int{}
	transactionsTimeseriesV1  = expvar.Int{}
	transactionsCheckRunsV1   = expvar.Int{}
	transactionsIntakeV1      = expvar.Int{}
)

func init() {
	transactionsExpvars.Init()
	forwarderExpvars.Set("Transactions", &transactionsExpvars)
	transactionsExpvars.Set("Series", &transactionsSeries)
	transactionsExpvars.Set("Events", &transactionsEvents)
	transactionsExpvars.Set("ServiceChecks", &transactionsServiceChecks)
	transactionsExpvars.Set("SketchSeries", &transactionsSketchSeries)
	transactionsExpvars.Set("HostMetadata", &transactionsHostMetadata)
	transactionsExpvars.Set("Metadata", &transactionsMetadata)
	transactionsExpvars.Set("TimeseriesV1", &transactionsTimeseriesV1)
	transactionsExpvars.Set("CheckRunsV1", &transactionsCheckRunsV1)
	transactionsExpvars.Set("IntakeV1", &transactionsIntakeV1)
	initDomainForwarderExpvars()
	initTransactionExpvars()
	initForwarderHealthExpvars()
}

const (
	// Stopped represent the internal state of an unstarted Forwarder.
	Stopped uint32 = iota
	// Started represent the internal state of an started Forwarder.
	Started
)

const (
	v1SeriesEndpoint       = "/api/v1/series"
	v1CheckRunsEndpoint    = "/api/v1/check_run"
	v1IntakeEndpoint       = "/intake/"
	v1SketchSeriesEndpoint = "/api/v1/sketches"
	v1ValidateEndpoint     = "/api/v1/validate"

	seriesEndpoint        = "/api/v2/series"
	eventsEndpoint        = "/api/v2/events"
	serviceChecksEndpoint = "/api/v2/service_checks"
	sketchSeriesEndpoint  = "/api/beta/sketches"
	hostMetadataEndpoint  = "/api/v2/host_metadata"
	metadataEndpoint      = "/api/v2/metadata"

	apiHTTPHeaderKey       = "DD-Api-Key"
	versionHTTPHeaderKey   = "DD-Agent-Version"
	useragentHTTPHeaderKey = "User-Agent"
)

// Payloads is a slice of pointers to byte arrays, an alias for the slices of
// payloads we pass into the forwarder
type Payloads []*[]byte

// Forwarder interface allows packages to send payload to the backend
type Forwarder interface {
	Start() error
	Stop()
	SubmitV1Series(payload Payloads, extra http.Header) error
	SubmitV1Intake(payload Payloads, extra http.Header) error
	SubmitV1CheckRuns(payload Payloads, extra http.Header) error
	SubmitSeries(payload Payloads, extra http.Header) error
	SubmitEvents(payload Payloads, extra http.Header) error
	SubmitServiceChecks(payload Payloads, extra http.Header) error
	SubmitSketchSeries(payload Payloads, extra http.Header) error
	SubmitHostMetadata(payload Payloads, extra http.Header) error
	SubmitMetadata(payload Payloads, extra http.Header) error
}

// DefaultForwarder is the default implementation of the Forwarder.
type DefaultForwarder struct {
	// NumberOfWorkers Number of concurrent HTTP request made by the DefaultForwarder (default 4).
	NumberOfWorkers int

	domainForwarders map[string]*domainForwarder
	keysPerDomains   map[string][]string
	healthChecker    *forwarderHealth
	internalState    uint32
	m                sync.Mutex // To control Start/Stop races
}

// NewDefaultForwarder returns a new DefaultForwarder.
func NewDefaultForwarder(keysPerDomains map[string][]string) *DefaultForwarder {
	f := &DefaultForwarder{
		NumberOfWorkers:  config.Datadog.GetInt("forwarder_num_workers"),
		domainForwarders: map[string]*domainForwarder{},
		keysPerDomains:   map[string][]string{},
		internalState:    Stopped,
		healthChecker:    &forwarderHealth{keysPerDomains: keysPerDomains},
	}
	numWorkers := config.Datadog.GetInt("forwarder_num_workers")
	retryQueueMaxSize := config.Datadog.GetInt("forwarder_retry_queue_max_size")

	for domain, keys := range keysPerDomains {
		domain, _ := config.AddAgentVersionToDomain(domain, "app")
		if keys == nil || len(keys) == 0 {
			log.Errorf("No API keys for domain '%s', dropping domain ", domain)
		} else {
			f.keysPerDomains[domain] = keys
			f.domainForwarders[domain] = newDomainForwarder(domain, numWorkers, retryQueueMaxSize)
		}
	}

	return f
}

// Start initialize and runs the forwarder.
func (f *DefaultForwarder) Start() error {
	// Lock so we can't stop a Forwarder while is starting
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState == Started {
		return fmt.Errorf("the forwarder is already started")
	}

	for _, df := range f.domainForwarders {
		df.Start()
	}

	// log endpoints configuration
	endpointLogs := make([]string, 0, len(f.keysPerDomains))
	for domain, apiKeys := range f.keysPerDomains {
		endpointLogs = append(endpointLogs, fmt.Sprintf("\"%s\" (%v api key(s))",
			domain, len(apiKeys)))
	}
	log.Infof("Forwarder started, sending to %v endpoint(s) with %v worker(s) each: %s",
		len(endpointLogs), f.NumberOfWorkers, strings.Join(endpointLogs, " ; "))

	f.healthChecker.Start()
	f.internalState = Started
	return nil
}

// Stop all the component of a forwarder and free resources
func (f *DefaultForwarder) Stop() {
	log.Infof("stopping the Forwarder")
	// Lock so we can't start a Forwarder while is stopping
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState == Stopped {
		log.Warnf("the forwarder is already stopped")
		return
	}

	f.internalState = Stopped

	purgeTimeout := config.Datadog.GetDuration("forwarder_stop_timeout") * time.Second
	if purgeTimeout > 0 {
		var wg sync.WaitGroup

		for _, df := range f.domainForwarders {
			wg.Add(1)
			go func(df *domainForwarder) {
				df.Stop(true)
				wg.Done()
			}(df)
		}

		donePurging := make(chan struct{})
		go func() {
			wg.Wait()
			close(donePurging)
		}()

		select {
		case <-donePurging:
		case <-time.After(purgeTimeout):
			log.Warnf("Timeout emptying new transactions before stopping the forwarder %v", purgeTimeout)
		}
	} else {
		for _, df := range f.domainForwarders {
			df.Stop(false)
		}
	}

	f.healthChecker.Stop()
	f.healthChecker = nil
	f.domainForwarders = map[string]*domainForwarder{}

}

// State returns the internal state of the forwarder (Started or Stopped)
func (f *DefaultForwarder) State() uint32 {
	// Lock so we can't start/stop a Forwarder while getting its state
	f.m.Lock()
	defer f.m.Unlock()

	return f.internalState
}

func (f *DefaultForwarder) createHTTPTransactions(endpoint string, payloads Payloads, apiKeyInQueryString bool, extra http.Header) []*HTTPTransaction {
	transactions := []*HTTPTransaction{}
	for _, payload := range payloads {
		for domain, apiKeys := range f.keysPerDomains {
			for _, apiKey := range apiKeys {
				transactionEndpoint := endpoint
				if apiKeyInQueryString {
					transactionEndpoint = fmt.Sprintf("%s?api_key=%s", endpoint, apiKey)
				}
				t := NewHTTPTransaction()
				t.Domain = domain
				t.Endpoint = transactionEndpoint
				t.Payload = payload
				t.Headers.Set(apiHTTPHeaderKey, apiKey)
				t.Headers.Set(versionHTTPHeaderKey, version.AgentVersion)
				t.Headers.Set(useragentHTTPHeaderKey, fmt.Sprintf("datadog-agent/%s", version.AgentVersion))

				for key := range extra {
					t.Headers.Set(key, extra.Get(key))
				}
				transactions = append(transactions, t)
			}
		}
	}
	return transactions
}

func (f *DefaultForwarder) sendHTTPTransactions(transactions []*HTTPTransaction) error {
	if atomic.LoadUint32(&f.internalState) == Stopped {
		return fmt.Errorf("the forwarder is not started")
	}

	for _, t := range transactions {
		if err := f.domainForwarders[t.Domain].sendHTTPTransactions(t); err != nil {
			log.Errorf(err.Error())
		}
	}
	return nil
}

// SubmitSeries will send a series type payload to Datadog backend.
func (f *DefaultForwarder) SubmitSeries(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(seriesEndpoint, payload, false, extra)
	transactionsSeries.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitEvents will send an event type payload to Datadog backend.
func (f *DefaultForwarder) SubmitEvents(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(eventsEndpoint, payload, false, extra)
	transactionsEvents.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitServiceChecks will send a service check type payload to Datadog backend.
func (f *DefaultForwarder) SubmitServiceChecks(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(serviceChecksEndpoint, payload, false, extra)
	transactionsServiceChecks.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitSketchSeries will send payloads to Datadog backend - PROTOTYPE FOR PERCENTILE
func (f *DefaultForwarder) SubmitSketchSeries(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(sketchSeriesEndpoint, payload, true, extra)
	transactionsSketchSeries.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *DefaultForwarder) SubmitHostMetadata(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(hostMetadataEndpoint, payload, false, extra)
	transactionsHostMetadata.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *DefaultForwarder) SubmitMetadata(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(metadataEndpoint, payload, false, extra)
	transactionsMetadata.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Series will send timeserie to v1 endpoint (this will be remove once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1Series(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(v1SeriesEndpoint, payload, true, extra)
	transactionsTimeseriesV1.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1CheckRuns will send service checks to v1 endpoint (this will be removed once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1CheckRuns(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(v1CheckRunsEndpoint, payload, true, extra)
	transactionsCheckRunsV1.Add(1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Intake will send payloads to the universal `/intake/` endpoint used by Agent v.5
func (f *DefaultForwarder) SubmitV1Intake(payload Payloads, extra http.Header) error {
	transactions := f.createHTTPTransactions(v1IntakeEndpoint, payload, true, extra)

	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}

	transactionsIntakeV1.Add(1)
	return f.sendHTTPTransactions(transactions)
}
