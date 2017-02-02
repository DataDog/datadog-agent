package forwarder

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"
)

var flushInterval = 5 * time.Second

const (
	defaultNumberOfWorkers = 4
	chanBufferSize         = 100

	seriesEndpoint       = "/api/v2/series"
	eventsEndpoint       = "/api/v2/events"
	checkRunsEndpoint    = "/api/v2/check_runs"
	hostMetadataEndpoint = "/api/v2/host_metadata"
	metadataEndpoint     = "/api/v2/metadata"
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
	// Endpoints are the different endpoints use to send Transaction.
	Domains []string
}

// NewForwarder returns a new Forwarder.
func NewForwarder(domains []string) *Forwarder {
	return &Forwarder{
		NumberOfWorkers: defaultNumberOfWorkers,
		Domains:         domains,
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
	// using atomic to stop createTransaction
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

func (f *Forwarder) createTransaction(endpoint string, payload *[]byte) error {
	if atomic.LoadUint32(&f.internalState) == Stopped {
		return fmt.Errorf("the forwarder is not started")
	}

	for _, domain := range f.Domains {
		t := NewHTTPTransaction()

		t.Domain = domain
		t.Endpoint = endpoint
		t.Payload = payload

		f.waitingPipe <- t
	}
	return nil
}

// SubmitTimeseries will send a timeserie type payload to Datadog backend.
func (f *Forwarder) SubmitTimeseries(payload *[]byte) error {
	return f.createTransaction(seriesEndpoint, payload)
}

// SubmitEvent will send a event type payload to Datadog backend.
func (f *Forwarder) SubmitEvent(payload *[]byte) error {
	return f.createTransaction(eventsEndpoint, payload)
}

// SubmitCheckRun will send a check_run type payload to Datadog backend.
func (f *Forwarder) SubmitCheckRun(payload *[]byte) error {
	return f.createTransaction(checkRunsEndpoint, payload)
}

// SubmitHostMetadata will send a host_metadata tag type payload to Datadog backend.
func (f *Forwarder) SubmitHostMetadata(payload *[]byte) error {
	return f.createTransaction(hostMetadataEndpoint, payload)
}

// SubmitMetadata will send a metadata type payload to Datadog backend.
func (f *Forwarder) SubmitMetadata(payload *[]byte) error {
	return f.createTransaction(metadataEndpoint, payload)
}
