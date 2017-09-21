// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package forwarder

import (
	"context"
	"expvar"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
)

var (
	flushInterval = 5 * time.Second

	forwarderExpvar      = expvar.NewMap("forwarder")
	transactionsCreation = expvar.Map{}
	retryQueueSize       = expvar.Int{}

	apiKeyStatus        = expvar.Map{}
	apiKeyStatusUnknown = expvar.String{}
	apiKeyInvalid       = expvar.String{}
	apiKeyValid         = expvar.String{}
)

func init() {
	transactionsCreation.Init()
	apiKeyStatus.Init()
	forwarderExpvar.Set("APIKeyStatus", &apiKeyStatus)
	forwarderExpvar.Set("TransactionsCreated", &transactionsCreation)
	transactionsCreation.Set("RetryQueueSize", &retryQueueSize)

	apiKeyStatusUnknown.Set("Unable to validate API Key")
	apiKeyInvalid.Set("API Key invalid")
	apiKeyValid.Set("API Key valid")
}

const (
	defaultNumberOfWorkers = 4
	chanBufferSize         = 100

	v1SeriesEndpoint       = "/api/v1/series"
	v1CheckRunsEndpoint    = "/api/v1/check_run"
	v1IntakeEndpoint       = "/intake/"
	v1SketchSeriesEndpoint = "/api/v1/sketches"

	seriesEndpoint        = "/api/v2/series"
	eventsEndpoint        = "/api/v2/events"
	serviceChecksEndpoint = "/api/v2/service_checks"
	sketchSeriesEndpoint  = "/api/beta/sketches"
	hostMetadataEndpoint  = "/api/v2/host_metadata"
	metadataEndpoint      = "/api/v2/metadata"

	apiHTTPHeaderKey = "DD-Api-Key"
)

const (
	// Stopped represent the internal state of an unstarted Forwarder.
	Stopped uint32 = iota
	// Started represent the internal state of an started Forwarder.
	Started
)

// Payloads is a slice of pointers to byte arrays, an alias for the slices of payloads we pass into the forwarder
type Payloads []*[]byte

// Transaction represents the task to process for a Worker.
type Transaction interface {
	Process(ctx context.Context, client *http.Client) error
	Reschedule()
	GetNextFlush() time.Time
	GetCreatedAt() time.Time
	GetTarget() string
}

// Forwarder implements basic interface - useful for testing
type Forwarder interface {
	Start() error
	Stop()
	SubmitV1Series(payload Payloads, extraHeaders map[string]string) error
	SubmitV1Intake(payload Payloads, extraHeaders map[string]string) error
	SubmitV1CheckRuns(payload Payloads, extraHeaders map[string]string) error
	SubmitSeries(payload Payloads, extraHeaders map[string]string) error
	SubmitEvents(payload Payloads, extraHeaders map[string]string) error
	SubmitServiceChecks(payload Payloads, extraHeaders map[string]string) error
	SubmitSketchSeries(payload Payloads, extraHeaders map[string]string) error
	SubmitHostMetadata(payload Payloads, extraHeaders map[string]string) error
	SubmitMetadata(payload Payloads, extraHeaders map[string]string) error
}

// DefaultForwarder is in charge of receiving transaction payloads and sending them to Datadog backend over HTTP.
type DefaultForwarder struct {
	waitingPipe         chan Transaction
	requeuedTransaction chan Transaction
	stopRetry           chan bool
	workers             []*Worker
	retryQueue          []Transaction
	internalState       uint32
	m                   sync.Mutex // To control Start/Stop races
	retryQueueLimit     int

	// NumberOfWorkers Number of concurrent HTTP request made by the DefaultForwarder (default 4).
	NumberOfWorkers int
	// KeysPerDomains are the different keys to use per domain when sending transactions.
	KeysPerDomains map[string][]string
}

// NewDefaultForwarder returns a new DefaultForwarder.
func NewDefaultForwarder(KeysPerDomains map[string][]string) *DefaultForwarder {
	return &DefaultForwarder{
		NumberOfWorkers: defaultNumberOfWorkers,
		KeysPerDomains:  KeysPerDomains,
		internalState:   Stopped,
		retryQueueLimit: config.Datadog.GetInt("forwarder_retry_queue_max_size"),
	}
}

type byCreatedTime []Transaction

func (v byCreatedTime) Len() int           { return len(v) }
func (v byCreatedTime) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v byCreatedTime) Less(i, j int) bool { return v[i].GetCreatedAt().After(v[j].GetCreatedAt()) }

func (f *DefaultForwarder) retryTransactions(tickTime time.Time) {
	newQueue := []Transaction{}
	droppedTransaction := 0

	sort.Sort(byCreatedTime(f.retryQueue))

	for _, t := range f.retryQueue {
		if t.GetNextFlush().Before(tickTime) {
			f.waitingPipe <- t
			transactionsCreation.Add("SuccessfullyRetried", 1)
		} else if len(newQueue) < f.retryQueueLimit {
			newQueue = append(newQueue, t)
		} else {
			transactionsCreation.Add("Dropped", 1)
			droppedTransaction++
		}
	}
	f.retryQueue = newQueue
	retryQueueSize.Set(int64(len(f.retryQueue)))
	if droppedTransaction != 0 {
		log.Warnf("forwarder retry queue size exceed limit from configuration (%d): dropped %d transactions (the oldest ones)", f.retryQueueLimit, droppedTransaction)
	}
}

func (f *DefaultForwarder) requeueTransaction(t Transaction) {
	f.retryQueue = append(f.retryQueue, t)
	transactionsCreation.Add("Requeued", 1)
}

func (f *DefaultForwarder) handleFailedTransactions() {
	ticker := time.NewTicker(flushInterval)
	for {
		select {
		case tickTime := <-ticker.C:
			f.retryTransactions(tickTime)
		case t := <-f.requeuedTransaction:
			f.requeueTransaction(t)
		case <-f.stopRetry:
			ticker.Stop()
			return
		}
	}
}

func (f *DefaultForwarder) init() {
	f.waitingPipe = make(chan Transaction, chanBufferSize)
	f.requeuedTransaction = make(chan Transaction, chanBufferSize)
	f.stopRetry = make(chan bool)
	f.workers = []*Worker{}
	f.retryQueue = []Transaction{}
}

// Start starts a DefaultForwarder.
func (f *DefaultForwarder) Start() error {
	// Lock so we can't stop a DefaultForwarder while is starting
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState == Started {
		return fmt.Errorf("the forwarder is already started")
	}

	// reset internal state to purge transactions from past starts
	f.init()

	blockedList := newBlockedEndpoints()
	for i := 0; i < f.NumberOfWorkers; i++ {
		w := NewWorker(f.waitingPipe, f.requeuedTransaction, blockedList)
		w.Start()
		f.workers = append(f.workers, w)
	}
	go f.handleFailedTransactions()
	f.internalState = Started
	log.Infof("DefaultForwarder started (%v workers)", f.NumberOfWorkers)

	// log endpoints configuration
	endpointLogs := make([]string, 0, len(f.KeysPerDomains))
	for domain, apiKeys := range f.KeysPerDomains {
		endpointLogs = append(endpointLogs, fmt.Sprintf("\"%s\" (%v api key(s))", domain, len(apiKeys)))
	}
	log.Infof("DefaultForwarder sending to %v endpoint(s): %s", len(endpointLogs), strings.Join(endpointLogs, " ; "))

	return nil
}

// State returns the internal state of the DefaultForwarder (either Started or Stopped).
func (f *DefaultForwarder) State() uint32 {
	return f.internalState
}

// Stop stops a DefaultForwarder, all transactions not yet flushed will be lost.
func (f *DefaultForwarder) Stop() {
	// Lock so we can't start a DefaultForwarder while is stopping
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState == Stopped {
		log.Errorf("the forwarder is already stopped")
		return
	}
	// using atomic to stop createTransactions
	atomic.StoreUint32(&f.internalState, Stopped)

	f.stopRetry <- true
	for _, w := range f.workers {
		w.Stop()
	}
	f.workers = []*Worker{}
	f.retryQueue = []Transaction{}
	close(f.waitingPipe)
	close(f.requeuedTransaction)
	log.Info("DefaultForwarder stopped")
}

func (f *DefaultForwarder) createHTTPTransactions(endpoint string, payloads Payloads, apiKeyInQueryString bool, extraHeaders map[string]string) ([]*HTTPTransaction, error) {
	transactions := []*HTTPTransaction{}
	for _, payload := range payloads {
		for domain, apiKeys := range f.KeysPerDomains {
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

				t.apiKeyStatusKey = fmt.Sprintf("%s,*************************", domain)
				if len(apiKey) > 5 {
					t.apiKeyStatusKey += apiKey[len(apiKey)-5:]
				}

				for k, v := range extraHeaders {
					t.Headers.Set(k, v)
				}
				transactions = append(transactions, t)
			}
		}
	}
	return transactions, nil
}

func (f *DefaultForwarder) sendHTTPTransactions(transactions []*HTTPTransaction) error {
	if atomic.LoadUint32(&f.internalState) == Stopped {
		return fmt.Errorf("the forwarder is not started")
	}

	for _, t := range transactions {
		f.waitingPipe <- t
	}

	return nil
}

// SubmitSeries will send a series type payload to Datadog backend.
func (f *DefaultForwarder) SubmitSeries(payload Payloads, extraHeaders map[string]string) error {
	transactions, err := f.createHTTPTransactions(seriesEndpoint, payload, false, extraHeaders)
	if err != nil {
		return err
	}

	transactionsCreation.Add("Series", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitEvents will send an event type payload to Datadog backend.
func (f *DefaultForwarder) SubmitEvents(payload Payloads, extraHeaders map[string]string) error {
	transactions, err := f.createHTTPTransactions(eventsEndpoint, payload, false, extraHeaders)
	if err != nil {
		return err
	}

	transactionsCreation.Add("Events", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitServiceChecks will send a service check type payload to Datadog backend.
func (f *DefaultForwarder) SubmitServiceChecks(payload Payloads, extraHeaders map[string]string) error {
	transactions, err := f.createHTTPTransactions(serviceChecksEndpoint, payload, false, extraHeaders)
	if err != nil {
		return err
	}

	transactionsCreation.Add("ServiceChecks", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitSketchSeries will send payloads to Datadog backend - PROTOTYPE FOR PERCENTILE
func (f *DefaultForwarder) SubmitSketchSeries(payload Payloads, extraHeaders map[string]string) error {
	transactions, err := f.createHTTPTransactions(sketchSeriesEndpoint, payload, true, extraHeaders)
	if err != nil {
		return err
	}
	transactionsCreation.Add("SketchSeries", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *DefaultForwarder) SubmitHostMetadata(payload Payloads, extraHeaders map[string]string) error {
	transactions, err := f.createHTTPTransactions(hostMetadataEndpoint, payload, false, extraHeaders)
	if err != nil {
		return err
	}

	transactionsCreation.Add("HostMetadata", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *DefaultForwarder) SubmitMetadata(payload Payloads, extraHeaders map[string]string) error {
	transactions, err := f.createHTTPTransactions(metadataEndpoint, payload, false, extraHeaders)
	if err != nil {
		return err
	}

	transactionsCreation.Add("Metadata", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Series will send timeserie to v1 endpoint (this will be remove once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1Series(payload Payloads, extraHeaders map[string]string) error {
	transactions, err := f.createHTTPTransactions(v1SeriesEndpoint, payload, true, extraHeaders)
	if err != nil {
		return err
	}

	transactionsCreation.Add("TimeseriesV1", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1CheckRuns will send service checks to v1 endpoint (this will be removed once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1CheckRuns(payload Payloads, extraHeaders map[string]string) error {
	transactions, err := f.createHTTPTransactions(v1CheckRunsEndpoint, payload, true, extraHeaders)
	if err != nil {
		return err
	}

	transactionsCreation.Add("CheckRunsV1", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Intake will send payloads to the universal `/intake/` endpoint used by Agent v.5
func (f *DefaultForwarder) SubmitV1Intake(payload Payloads, extraHeaders map[string]string) error {
	transactions, err := f.createHTTPTransactions(v1IntakeEndpoint, payload, true, extraHeaders)
	if err != nil {
		return err
	}

	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}

	transactionsCreation.Add("IntakeV1", 1)
	return f.sendHTTPTransactions(transactions)
}
