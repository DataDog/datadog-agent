// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/internal/retry"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

var (
	flushInterval = 5 * time.Second
)

// domainForwarder is in charge of sending Transactions to Datadog backend over
// HTTP and retrying them if needed. One domainForwarder is created per HTTP
// backend.
type domainForwarder struct {
	config                    config.Component
	log                       log.Component
	isRetrying                *atomic.Bool
	domain                    string
	numberOfWorkers           int
	highPrio                  chan transaction.Transaction // use to receive new transactions
	lowPrio                   chan transaction.Transaction // use to retry transactions
	requeuedTransaction       chan transaction.Transaction
	stopRetry                 chan bool
	stopConnectionReset       chan bool
	workers                   []*Worker
	retryQueue                *retry.TransactionRetryQueue
	connectionResetInterval   time.Duration
	internalState             uint32
	m                         sync.Mutex // To control Start/Stop races
	transactionPrioritySorter retry.TransactionPrioritySorter
	blockedList               *blockedEndpoints
	pointCountTelemetry       *retry.PointCountTelemetry
}

func newDomainForwarder(
	config config.Component,
	log log.Component,
	domain string,
	retryQueue *retry.TransactionRetryQueue,
	numberOfWorkers int,
	connectionResetInterval time.Duration,
	transactionPrioritySorter retry.TransactionPrioritySorter,
	pointCountTelemetry *retry.PointCountTelemetry) *domainForwarder {
	return &domainForwarder{
		config:                    config,
		log:                       log,
		isRetrying:                atomic.NewBool(false),
		domain:                    domain,
		numberOfWorkers:           numberOfWorkers,
		retryQueue:                retryQueue,
		connectionResetInterval:   connectionResetInterval,
		internalState:             Stopped,
		blockedList:               newBlockedEndpoints(config, log),
		transactionPrioritySorter: transactionPrioritySorter,
		pointCountTelemetry:       pointCountTelemetry,
	}
}

func (f *domainForwarder) retryTransactions(retryBefore time.Time) {
	// In case it takes more that flushInterval to sort and retry
	// transactions we skip a retry.
	if !f.isRetrying.CompareAndSwap(false, true) {
		f.log.Errorf("The forwarder is still retrying Transaction: this should never happens, you might want to lower the 'forwarder_retry_queue_payloads_max_size'")
		return
	}
	defer f.isRetrying.Store(false)

	droppedRetryQueueFull := 0
	droppedWorkerBusy := 0

	var transactions []transaction.Transaction
	var err error

	transactions, err = f.retryQueue.ExtractTransactions()
	if err != nil {
		f.log.Errorf("Error when getting transactions from the retry queue: %v", err)
	}

	f.transactionPrioritySorter.Sort(transactions)

	for _, t := range transactions {
		transactionEndpointName := t.GetEndpointName()
		if !f.blockedList.isBlock(t.GetTarget()) {
			select {
			case f.lowPrio <- t:
				transactionsRetriedByEndpoint.Add(transactionEndpointName, 1)
				transactionsRetried.Add(1)
				tlmTxRetried.Inc(f.domain, transactionEndpointName)
			default:
				dropCount := f.addToTransactionRetryQueue(t)
				tlmTxRequeued.Inc(f.domain, transactionEndpointName)
				droppedWorkerBusy += dropCount
			}
		} else {
			dropCount := f.addToTransactionRetryQueue(t)
			transactionsRequeued.Add(1)
			tlmTxRequeued.Inc(f.domain, transactionEndpointName)
			droppedRetryQueueFull += dropCount
		}
	}

	transactionCount := f.retryQueue.GetTransactionCount()
	transactionsRetryQueueSize.Set(int64(transactionCount))
	tlmTxRetryQueueSize.Set(float64(transactionCount), f.domain)

	if droppedRetryQueueFull+droppedWorkerBusy > 0 {
		f.log.Errorf("Dropped %d transactions in this retry attempt:%d for exceeding the retry queue payloads size limit of %d, %d because the workers are too busy",
			droppedRetryQueueFull+droppedWorkerBusy, droppedRetryQueueFull, f.retryQueue.GetMaxMemSizeInBytes(), droppedWorkerBusy)
	}
}

func (f *domainForwarder) addToTransactionRetryQueue(t transaction.Transaction) int {
	dropCount, err := f.retryQueue.Add(t)
	if err != nil {
		f.log.Errorf("Error when adding a transaction to the retry queue: %v", err)
	}

	if dropCount > 0 {
		transactionEndpointName := t.GetEndpointName()
		transaction.TransactionsDroppedByEndpoint.Add(transactionEndpointName, int64(dropCount))
		transaction.TransactionsDropped.Add(int64(dropCount))
		transaction.TlmTxDropped.Inc(f.domain, transactionEndpointName)
	}
	return dropCount
}

func (f *domainForwarder) requeueTransaction(t transaction.Transaction) {
	f.addToTransactionRetryQueue(t)
	retryQueueSize := f.retryQueue.GetTransactionCount()
	transactionsRequeuedByEndpoint.Add(t.GetEndpointName(), 1)
	transactionsRequeued.Add(1)
	transactionsRetryQueueSize.Set(int64(retryQueueSize))
	tlmTxRetryQueueSize.Set(float64(retryQueueSize), f.domain)
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
			f.log.Debugf("Scheduling reset of connections used for domain: %q", f.domain)
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
	highPrioBuffSize := f.config.GetInt("forwarder_high_prio_buffer_size")
	lowPrioBuffSize := f.config.GetInt("forwarder_low_prio_buffer_size")
	requeuedTransactionBuffSize := f.config.GetInt("forwarder_requeue_buffer_size")

	f.highPrio = make(chan transaction.Transaction, highPrioBuffSize)
	f.lowPrio = make(chan transaction.Transaction, lowPrioBuffSize)
	f.requeuedTransaction = make(chan transaction.Transaction, requeuedTransactionBuffSize)
	f.stopRetry = make(chan bool)
	f.stopConnectionReset = make(chan bool)
	f.workers = []*Worker{}
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
		w := NewWorker(f.config, f.log, f.highPrio, f.lowPrio, f.requeuedTransaction, f.blockedList, f.pointCountTelemetry)
		w.Start()
		f.workers = append(f.workers, w)
	}
	go f.handleFailedTransactions()
	if f.connectionResetInterval != 0 {
		go f.scheduleConnectionResets()
	}

	f.pointCountTelemetry.Start()
	f.internalState = Started
	return nil
}

// Stop stops a domainForwarder, all transactions not yet flushed will be lost.
func (f *domainForwarder) Stop(purgeHighPrio bool) {
	// Lock so we can't start a Forwarder while is stopping
	f.m.Lock()
	defer f.m.Unlock()

	if f.internalState == Stopped {
		f.log.Warnf("the forwarder is already stopped")
		return
	}

	f.pointCountTelemetry.Stop()

	if f.connectionResetInterval != 0 {
		f.stopConnectionReset <- true
	}
	f.stopRetry <- true
	for _, w := range f.workers {
		w.Stop(purgeHighPrio)
	}
	f.workers = []*Worker{}
	close(f.highPrio)
	close(f.lowPrio)
	close(f.requeuedTransaction)

	for t := range f.requeuedTransaction {
		f.requeueTransaction(t)
	}
	if err := f.retryQueue.FlushToDisk(); err != nil {
		f.log.Errorf("Error when flushing the retry queue to disk: %v", err)
	}

	f.log.Info("domainForwarder stopped")
	f.internalState = Stopped
}

func (f *domainForwarder) State() uint32 {
	// Lock so we can't start/stop a Forwarder while getting its state
	f.m.Lock()
	defer f.m.Unlock()

	return f.internalState
}

func (f *domainForwarder) sendHTTPTransactions(t transaction.Transaction) {
	// We don't want to block the collector if the highPrio queue is full
	select {
	case f.highPrio <- t:
	default:
		f.addToTransactionRetryQueue(t)
		highPriorityQueueFull.Add(1)
		tlmTxHighPriorityQueueFull.Inc(f.domain, t.GetEndpointName())
		f.log.Debugf("Adding the transaction to the retry queue because the forwarder input queue for %s is full; consider increasing forwarder_num_workers", f.domain)
	}
}
