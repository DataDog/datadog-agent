// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package forwarder

import (
	"expvar"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	chanBufferSize = 100
	flushInterval  = 5 * time.Second

	transactionsRetried  = expvar.Int{}
	transactionsDropped  = expvar.Int{}
	transactionsRequeued = expvar.Int{}

	tlmTxRetried = telemetry.NewCounter("transactions", "retries",
		[]string{"domain"}, "Transaction retry count")
	tlmTxDropped = telemetry.NewCounter("transactions", "dropped",
		[]string{"domain"}, "Transaction drop count")
	tlmTxRequeud = telemetry.NewCounter("transactions", "requeud",
		[]string{"domain"}, "Transaction requeue count")
)

func initDomainForwarderExpvars() {
	transactionsExpvars.Set("Retried", &transactionsRetried)
	transactionsExpvars.Set("Dropped", &transactionsDropped)
	transactionsExpvars.Set("Requeued", &transactionsRequeued)
}

// domainForwarder is in charge of sending Transactions to Datadog backend over
// HTTP and retrying them if needed. One domainForwarder is created per HTTP
// backend.
type domainForwarder struct {
	isRetrying              int32
	domain                  string
	numberOfWorkers         int
	highPrio                chan Transaction // use to receive new transactions
	lowPrio                 chan Transaction // use to retry transactions
	requeuedTransaction     chan Transaction
	stopRetry               chan bool
	stopConnectionReset     chan bool
	workers                 []*Worker
	retryQueue              []Transaction
	retryQueueLimit         int
	connectionResetInterval time.Duration
	internalState           uint32
	m                       sync.Mutex // To control Start/Stop races

	blockedList *blockedEndpoints
}

func newDomainForwarder(domain string, numberOfWorkers int, retryQueueLimit int, connectionResetInterval time.Duration) *domainForwarder {
	return &domainForwarder{
		domain:                  domain,
		numberOfWorkers:         numberOfWorkers,
		retryQueueLimit:         retryQueueLimit,
		connectionResetInterval: connectionResetInterval,
		internalState:           Stopped,
		blockedList:             newBlockedEndpoints(),
	}
}

type byCreatedTime []Transaction

func (v byCreatedTime) Len() int           { return len(v) }
func (v byCreatedTime) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v byCreatedTime) Less(i, j int) bool { return v[i].GetCreatedAt().After(v[j].GetCreatedAt()) }

func (f *domainForwarder) retryTransactions(retryBefore time.Time) {
	// In case it takes more that flushInterval to sort and retry
	// transactions we skip a retry.
	if !atomic.CompareAndSwapInt32(&f.isRetrying, 0, 1) {
		log.Errorf("The forwarder is still retrying Transaction: this should never happens and you might lower the 'forwarder_retry_queue_max_size'")
		return
	}
	defer atomic.StoreInt32(&f.isRetrying, 0)

	newQueue := []Transaction{}
	droppedRetryQueueFull := 0
	droppedWorkerBusy := 0

	sort.Sort(byCreatedTime(f.retryQueue))

	for _, t := range f.retryQueue {
		if !f.blockedList.isBlock(t.GetTarget()) {
			select {
			case f.lowPrio <- t:
				transactionsRetried.Add(1)
				tlmTxRetried.Inc(f.domain)
			default:
				droppedWorkerBusy++
				transactionsDropped.Add(1)
				tlmTxDropped.Inc(f.domain)
			}
		} else if len(newQueue) < f.retryQueueLimit {
			newQueue = append(newQueue, t)
			transactionsRequeued.Add(1)
			tlmTxRequeud.Inc(f.domain)
		} else {
			droppedRetryQueueFull++
			transactionsDropped.Add(1)
			tlmTxDropped.Inc(f.domain)
		}
	}

	f.retryQueue = newQueue
	transactionsRetryQueueSize.Set(int64(len(f.retryQueue)))
	tlmTxRetryQueueSize.Set(float64(len(f.retryQueue)), f.domain)

	if droppedRetryQueueFull+droppedWorkerBusy > 0 {
		log.Errorf("Dropped %d transactions in this retry attempt: %d for exceeding the retry queue size limit of %d, %d because the workers are too busy",
			droppedRetryQueueFull+droppedWorkerBusy, droppedRetryQueueFull, f.retryQueueLimit, droppedWorkerBusy)
	}
}

func (f *domainForwarder) requeueTransaction(t Transaction) {
	f.retryQueue = append(f.retryQueue, t)
	transactionsRequeued.Add(1)
	transactionsRetryQueueSize.Set(int64(len(f.retryQueue)))
	tlmTxRetryQueueSize.Set(float64(len(f.retryQueue)), f.domain)
}

func (f *domainForwarder) handleFailedTransactions() {
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

// scheduleConnectionResets signals the workers to recreate their connections to DD
// at the configured interval
func (f *domainForwarder) scheduleConnectionResets() {
	ticker := time.NewTicker(f.connectionResetInterval)
	for {
		select {
		case <-ticker.C:
			log.Debugf("Scheduling reset of connections used for domain: %q", f.domain)
			for _, worker := range f.workers {
				worker.ScheduleConnectionReset()
			}
		case <-f.stopConnectionReset:
			ticker.Stop()
			return
		}
	}
}

func (f *domainForwarder) init() {
	f.highPrio = make(chan Transaction, chanBufferSize)
	f.lowPrio = make(chan Transaction, chanBufferSize)
	f.requeuedTransaction = make(chan Transaction, chanBufferSize)
	f.stopRetry = make(chan bool)
	f.stopConnectionReset = make(chan bool)
	f.workers = []*Worker{}
	f.retryQueue = []Transaction{}
}

// Start starts a domainForwarder.
func (f *domainForwarder) Start() error {
	// Lock so we can't stop a Forwarder while is starting
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState == Started {
		return fmt.Errorf("the forwarder is already started")
	}

	// reset internal state to purge transactions from past starts
	f.init()

	for i := 0; i < f.numberOfWorkers; i++ {
		w := NewWorker(f.highPrio, f.lowPrio, f.requeuedTransaction, f.blockedList)
		w.Start()
		f.workers = append(f.workers, w)
	}
	go f.handleFailedTransactions()
	if f.connectionResetInterval != 0 {
		go f.scheduleConnectionResets()
	}

	f.internalState = Started
	return nil
}

// Stop stops a domainForwarder, all transactions not yet flushed will be lost.
func (f *domainForwarder) Stop(purgeHighPrio bool) {
	// Lock so we can't start a Forwarder while is stopping
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState == Stopped {
		log.Warnf("the forwarder is already stopped")
		return
	}

	if f.connectionResetInterval != 0 {
		f.stopConnectionReset <- true
	}
	f.stopRetry <- true
	for _, w := range f.workers {
		w.Stop(purgeHighPrio)
	}
	f.workers = []*Worker{}
	f.retryQueue = []Transaction{}
	close(f.highPrio)
	close(f.lowPrio)
	close(f.requeuedTransaction)
	log.Info("domainForwarder stopped")
	f.internalState = Stopped
}

func (f *domainForwarder) State() uint32 {
	// Lock so we can't start/stop a Forwarder while getting its state
	f.m.Lock()
	defer f.m.Unlock()

	return f.internalState
}

func (f *domainForwarder) sendHTTPTransactions(transaction Transaction) error {
	// We don't want to block the collector if the highPrio queue is full
	select {
	case f.highPrio <- transaction:
	default:
		transactionsDroppedOnInput.Add(1)
		tlmTxDroppedOnInput.Inc(f.domain)
		return fmt.Errorf("the forwarder input queue for %s is full: dropping transaction", f.domain)
	}
	return nil
}
