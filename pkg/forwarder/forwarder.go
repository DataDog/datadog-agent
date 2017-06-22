package forwarder

import (
	"expvar"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
	log "github.com/cihub/seelog"
)

var (
	flushInterval = 5 * time.Second

	forwarderExpvar      = expvar.NewMap("forwarder")
	transactionsCreation = expvar.Map{}
)

func init() {
	transactionsCreation.Init()
	forwarderExpvar.Set("TransactionsCreated", &transactionsCreation)
}

const (
	defaultNumberOfWorkers = 4
	chanBufferSize         = 100

	v1SeriesEndpoint       = "/api/v1/series?api_key=%s"
	v1CheckRunsEndpoint    = "/api/v1/check_run?api_key=%s"
	v1IntakeEndpoint       = "/intake/?api_key=%s"
	v1SketchSeriesEndpoint = "/api/v1/sketches?api_key=%s"

	seriesEndpoint       = "/api/v2/series"
	eventsEndpoint       = "/api/v2/events"
	checkRunsEndpoint    = "/api/v2/check_runs"
	hostMetadataEndpoint = "/api/v2/host_metadata"
	metadataEndpoint     = "/api/v2/metadata"

	apiHTTPHeaderKey = "DD-Api-Key"
)

const (
	// Stopped represent the internal state of an unstarted Forwarder.
	Stopped uint32 = iota
	// Started represent the internal state of an started Forwarder.
	Started
)

// Transaction represents the task to process for a Worker.
type Transaction interface {
	Process(client *http.Client) error
	Reschedule()
	GetNextFlush() time.Time
	GetCreatedAt() time.Time
}

// Forwarder implements basic interface - useful for testing
type Forwarder interface {
	Start() error
	Stop()
	SubmitV1Series(apiKey string, payload *[]byte) error
	SubmitV1Intake(apiKey string, payload *[]byte) error
	SubmitV1CheckRuns(apiKey string, payload *[]byte) error
	SubmitV2Series(apikey string, payload *[]byte) error
	SubmitV2Events(apikey string, payload *[]byte) error
	SubmitV2CheckRuns(apikey string, payload *[]byte) error
	SubmitV2HostMeta(apikey string, payload *[]byte) error
	SubmitV2GenericMeta(apikey string, payload *[]byte) error
	SubmitV1SketchSeries(apiKey string, payload *[]byte) error
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
	}
}

type byCreatedTime []Transaction

func (v byCreatedTime) Len() int           { return len(v) }
func (v byCreatedTime) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v byCreatedTime) Less(i, j int) bool { return v[i].GetCreatedAt().After(v[j].GetCreatedAt()) }

func (f *DefaultForwarder) retryTransactions(tickTime time.Time) {
	newQueue := []Transaction{}

	sort.Sort(byCreatedTime(f.retryQueue))

	for _, t := range f.retryQueue {
		if t.GetNextFlush().Before(tickTime) {
			f.waitingPipe <- t
			transactionsCreation.Add("SuccessfullyRetried", 1)
		} else {
			newQueue = append(newQueue, t)
		}
	}
	f.retryQueue = newQueue
	transactionsCreation.Add("RetryQueueSize", int64(len(f.retryQueue)))
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

	for i := 0; i < f.NumberOfWorkers; i++ {
		w := NewWorker(f.waitingPipe, f.requeuedTransaction)
		w.Start()
		f.workers = append(f.workers, w)
	}
	go f.handleFailedTransactions()
	f.internalState = Started
	log.Infof("DefaultForwarder started (%v workers)", f.NumberOfWorkers)
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

func (f *DefaultForwarder) createHTTPTransactions(endpoint string, payload *[]byte, compress bool) ([]*HTTPTransaction, error) {
	if compress && payload != nil {
		compressPayload, err := compression.Compress(nil, *payload)
		if err != nil {
			return nil, err
		}
		payload = &compressPayload
	}

	transactions := []*HTTPTransaction{}
	for domain, apiKeys := range f.KeysPerDomains {
		for _, apiKey := range apiKeys {
			t := NewHTTPTransaction()
			t.Domain = domain
			t.Endpoint = endpoint
			t.Payload = payload
			t.Headers.Set(apiHTTPHeaderKey, apiKey)
			transactions = append(transactions, t)
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

// SubmitTimeseries will send a timeserie type payload to Datadog backend.
func (f *DefaultForwarder) SubmitTimeseries(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(seriesEndpoint, payload, true)
	if err != nil {
		return err
	}

	transactionsCreation.Add("Timeseries", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitEvent will send a event type payload to Datadog backend.
func (f *DefaultForwarder) SubmitEvent(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(eventsEndpoint, payload, true)
	if err != nil {
		return err
	}

	transactionsCreation.Add("Events", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitCheckRun will send a check_run type payload to Datadog backend.
func (f *DefaultForwarder) SubmitCheckRun(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(checkRunsEndpoint, payload, true)
	if err != nil {
		return err
	}

	transactionsCreation.Add("CheckRuns", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *DefaultForwarder) SubmitHostMetadata(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(hostMetadataEndpoint, payload, true)
	if err != nil {
		return err
	}

	transactionsCreation.Add("HostMetadata", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *DefaultForwarder) SubmitMetadata(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(metadataEndpoint, payload, true)
	if err != nil {
		return err
	}

	transactionsCreation.Add("Metadata", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Series will send timeserie to v1 endpoint (this will be remove once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1Series(apiKey string, payload *[]byte) error {
	endpoint := fmt.Sprintf(v1SeriesEndpoint, apiKey)
	transactions, err := f.createHTTPTransactions(endpoint, payload, false)
	if err != nil {
		return err
	}

	transactionsCreation.Add("TimeseriesV1", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1CheckRuns will send service checks to v1 endpoint (this will be removed once
// the backend handles v2 endpoints).
func (f *DefaultForwarder) SubmitV1CheckRuns(apiKey string, payload *[]byte) error {
	endpoint := fmt.Sprintf(v1CheckRunsEndpoint, apiKey)
	transactions, err := f.createHTTPTransactions(endpoint, payload, false)
	if err != nil {
		return err
	}

	transactionsCreation.Add("CheckRunsV1", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Intake will send payloads to the universal `/intake/` endpoint used by Agent v.5
func (f *DefaultForwarder) SubmitV1Intake(apiKey string, payload *[]byte) error {
	endpoint := fmt.Sprintf(v1IntakeEndpoint, apiKey)
	transactions, err := f.createHTTPTransactions(endpoint, payload, false)
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

// SubmitV1SketchSeries will send payloads to v1 endpoint
func (f *DefaultForwarder) SubmitV1SketchSeries(apiKey string, payload *[]byte) error {
	endpoint := fmt.Sprintf(v1SketchSeriesEndpoint, apiKey)
	transactions, err := f.createHTTPTransactions(endpoint, payload, false)
	if err != nil {
		return err
	}
	transactionsCreation.Add("SketchSeries", 1)
	return f.sendHTTPTransactions(transactions)
}

// SubmitV2Series will send service checks to v2 endpoint - UNIMPLEMENTED
func (f *DefaultForwarder) SubmitV2Series(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
}

// SubmitV2Events will send events to v2 endpoint - UNIMPLEMENTED
func (f *DefaultForwarder) SubmitV2Events(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
}

// SubmitV2CheckRuns will send service checks to v2 endpoint - UNIMPLEMENTED
func (f *DefaultForwarder) SubmitV2CheckRuns(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
}

// SubmitV2HostMeta will send host metadata to v2 endpoint - UNIMPLEMENTED
func (f *DefaultForwarder) SubmitV2HostMeta(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
}

// SubmitV2GenericMeta will send generic metadata to v2 endpoint - UNIMPLEMENTED
func (f *DefaultForwarder) SubmitV2GenericMeta(apikey string, payload *[]byte) error {
	return fmt.Errorf("v2 endpoint submission unimplemented")
}
