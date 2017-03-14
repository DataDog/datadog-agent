package forwarder

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/zstd"
)

var flushInterval = 5 * time.Second

const (
	defaultNumberOfWorkers = 4
	chanBufferSize         = 100

	v1SeriesEndpoint    = "/api/v1/series?api_key=%s"
	v1CheckRunsEndpoint = "/api/v1/check_run?api_key=%s"
	v1IntakeEndpoint    = "/intake/?api_key=%s"

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

// Forwarder is in charge of receiving transaction payloads and sending them to Datadog backend over HTTP.
type Forwarder struct {
	waitingPipe         chan Transaction
	requeuedTransaction chan Transaction
	stopRetry           chan bool
	workers             []*Worker
	retryQueue          []Transaction
	internalState       uint32
	m                   sync.Mutex // To control Start/Stop races

	// NumberOfWorkers Number of concurrent HTTP request made by the Forwarder (default 4).
	NumberOfWorkers int
	// KeysPerDomains are the different keys to use per domain when sending transactions.
	KeysPerDomains map[string][]string
}

// NewForwarder returns a new Forwarder.
func NewForwarder(KeysPerDomains map[string][]string) *Forwarder {
	return &Forwarder{
		NumberOfWorkers: defaultNumberOfWorkers,
		KeysPerDomains:  KeysPerDomains,
		internalState:   Stopped,
	}
}

type byCreatedTime []Transaction

func (v byCreatedTime) Len() int           { return len(v) }
func (v byCreatedTime) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v byCreatedTime) Less(i, j int) bool { return v[i].GetCreatedAt().After(v[j].GetCreatedAt()) }

func (f *Forwarder) retryTransactions(tickTime time.Time) {
	newQueue := []Transaction{}

	sort.Sort(byCreatedTime(f.retryQueue))

	for _, t := range f.retryQueue {
		if t.GetNextFlush().Before(tickTime) {
			f.waitingPipe <- t
		} else {
			newQueue = append(newQueue, t)
		}
	}
	f.retryQueue = newQueue
}

func (f *Forwarder) requeueTransaction(t Transaction) {
	f.retryQueue = append(f.retryQueue, t)
}

func (f *Forwarder) handleFailedTransactions() {
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

func (f *Forwarder) init() {
	f.waitingPipe = make(chan Transaction, chanBufferSize)
	f.requeuedTransaction = make(chan Transaction, chanBufferSize)
	f.stopRetry = make(chan bool)
	f.workers = []*Worker{}
	f.retryQueue = []Transaction{}
}

// Start starts a Forwarder.
func (f *Forwarder) Start() error {
	// Lock so we can't stop a Forwarder while is starting
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
	log.Infof("Forwarder started (%v workers)", f.NumberOfWorkers)
	return nil
}

// State returns the internal state of the Forwarder (either Started or Stopped).
func (f *Forwarder) State() uint32 {
	return f.internalState
}

// Stop stops a Forwarder, all transactions not yet flushed will be lost.
func (f *Forwarder) Stop() {
	// Lock so we can't start a Forwarder while is stopping
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
	log.Info("Forwarder stopped")
}

func (f *Forwarder) createHTTPTransactions(endpoint string, payload *[]byte, compress bool) ([]*HTTPTransaction, error) {
	if compress && payload != nil {
		compressPayload, err := zstd.Compress(nil, *payload)
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

func (f *Forwarder) sendHTTPTransactions(transactions []*HTTPTransaction) error {
	if atomic.LoadUint32(&f.internalState) == Stopped {
		return fmt.Errorf("the forwarder is not started")
	}

	for _, t := range transactions {
		f.waitingPipe <- t
	}

	return nil
}

// SubmitTimeseries will send a timeserie type payload to Datadog backend.
func (f *Forwarder) SubmitTimeseries(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(seriesEndpoint, payload, true)
	if err != nil {
		return err
	}

	return f.sendHTTPTransactions(transactions)
}

// SubmitEvent will send a event type payload to Datadog backend.
func (f *Forwarder) SubmitEvent(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(eventsEndpoint, payload, true)
	if err != nil {
		return err
	}

	return f.sendHTTPTransactions(transactions)
}

// SubmitCheckRun will send a check_run type payload to Datadog backend.
func (f *Forwarder) SubmitCheckRun(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(checkRunsEndpoint, payload, true)
	if err != nil {
		return err
	}

	return f.sendHTTPTransactions(transactions)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *Forwarder) SubmitHostMetadata(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(hostMetadataEndpoint, payload, true)
	if err != nil {
		return err
	}

	return f.sendHTTPTransactions(transactions)
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *Forwarder) SubmitMetadata(payload *[]byte) error {
	transactions, err := f.createHTTPTransactions(metadataEndpoint, payload, true)
	if err != nil {
		return err
	}

	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Series will send timeserie to v1 endpoint (this will be remove once
// the backend handles v2 endpoints).
func (f *Forwarder) SubmitV1Series(apiKey string, payload *[]byte) error {
	endpoint := fmt.Sprintf(v1SeriesEndpoint, apiKey)
	transactions, err := f.createHTTPTransactions(endpoint, payload, false)
	if err != nil {
		return err
	}

	return f.sendHTTPTransactions(transactions)
}

// SubmitV1CheckRuns will send service checks to v1 endpoint (this will be removed once
// the backend handles v2 endpoints).
func (f *Forwarder) SubmitV1CheckRuns(apiKey string, payload *[]byte) error {
	endpoint := fmt.Sprintf(v1CheckRunsEndpoint, apiKey)
	transactions, err := f.createHTTPTransactions(endpoint, payload, false)
	if err != nil {
		return err
	}

	return f.sendHTTPTransactions(transactions)
}

// SubmitV1Intake will send payloads to the universal `/intake/` endpoint used by Agent v.5
func (f *Forwarder) SubmitV1Intake(apiKey string, payload *[]byte) error {
	endpoint := fmt.Sprintf(v1IntakeEndpoint, apiKey)
	transactions, err := f.createHTTPTransactions(endpoint, payload, false)
	if err != nil {
		return err
	}

	// the intake endpoint requires the Content-Type header to be set
	for _, t := range transactions {
		t.Headers.Set("Content-Type", "application/json")
	}

	return f.sendHTTPTransactions(transactions)
}
