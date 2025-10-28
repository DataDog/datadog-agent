// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"context"
	"fmt"
	"net/http/httptrace"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

// Worker consumes Transaction (aka transactions) from the Forwarder and
// processes them. If the transaction fails to be processed the Worker will send
// it back to the Forwarder to be retried later.
type Worker struct {
	config config.Component
	log    log.Component

	// Client the http client used to processed transactions.
	Client *SharedConnection
	// HighPrio is the channel used to receive high priority transaction from the Forwarder.
	HighPrio <-chan transaction.Transaction
	// LowPrio is the channel used to receive low priority transaction from the Forwarder.
	LowPrio <-chan transaction.Transaction
	// RequeueChan is the channel used to send failed transaction back to the Forwarder.
	RequeueChan chan<- transaction.Transaction

	stopped               chan struct{}
	blockedList           *blockedEndpoints
	pointSuccessfullySent PointSuccessfullySent

	// The maximum number of HTTP requests we can have inflight at any one time.
	maxConcurrentRequests *semaphore.Weighted
	workerCtx             context.Context
	cancel                context.CancelFunc
	requestWg             sync.WaitGroup
}

// PointSuccessfullySent is called when sending successfully a point to the intake.
type PointSuccessfullySent interface {
	OnPointSuccessfullySent(int)
}

// NewWorker returns a new worker to consume Transaction from inputChan
// and push back erroneous ones into requeueChan.
func NewWorker(
	config config.Component,
	log log.Component,
	highPrioChan <-chan transaction.Transaction,
	lowPrioChan <-chan transaction.Transaction,
	requeueChan chan<- transaction.Transaction,
	blocked *blockedEndpoints,
	pointSuccessfullySent PointSuccessfullySent,
	httpClient *SharedConnection,
) *Worker {
	maxConcurrentRequests := config.GetInt64("forwarder_max_concurrent_requests")
	if maxConcurrentRequests <= 0 {
		log.Warnf("Invalid forwarder_max_concurrent_requests '%d', setting to 1", maxConcurrentRequests)
		maxConcurrentRequests = 1
	}

	workerCtx, cancel := context.WithCancel(context.Background())

	worker := &Worker{
		config:                config,
		log:                   log,
		HighPrio:              highPrioChan,
		LowPrio:               lowPrioChan,
		RequeueChan:           requeueChan,
		stopped:               make(chan struct{}),
		Client:                httpClient,
		blockedList:           blocked,
		pointSuccessfullySent: pointSuccessfullySent,
		maxConcurrentRequests: semaphore.NewWeighted(maxConcurrentRequests),
		workerCtx:             workerCtx,
		cancel:                cancel,
	}
	return worker
}

// Stop stops the worker.
func (w *Worker) Stop(purgeHighPrio bool) {
	// Cancel our context to kick out any transactions waiting
	// on the maxConcurrentRequests semaphore.
	w.cancel()

	<-w.stopped

	if purgeHighPrio {
		// Need a new context to flush these high priority transactions.
		w.workerCtx, w.cancel = context.WithCancel(context.Background())
	L:
		for {
			select {
			case t := <-w.HighPrio:
				w.log.Debugf("Flushing one new transaction before stopping Worker")
				w.callProcess(t) //nolint:errcheck
			default:
				break L
			}
		}
	}

	w.requestWg.Wait()
}

// Start starts a Worker.
func (w *Worker) Start() {
	go func() {
		// notify that the worker did stop
		defer close(w.stopped)

		for {
			// handling high priority transactions first
			select {
			case t := <-w.HighPrio:
				if w.callProcess(t) == nil {
					continue
				}
				return
			case <-w.workerCtx.Done():
				return
			default:
			}

			select {
			case t := <-w.HighPrio:
				if w.callProcess(t) != nil {
					return
				}
			case t := <-w.LowPrio:
				if w.callProcess(t) != nil {
					return
				}
			case <-w.workerCtx.Done():
				return
			}
		}
	}()
}

// acquireRequestSemaphore attempts to acquire a semaphore, which will block
// if we are already sending too many requests.
func (w *Worker) acquireRequestSemaphore(ctx context.Context) error {
	err := w.maxConcurrentRequests.Acquire(ctx, 1)
	if err != nil {
		return fmt.Errorf("unable to acquire request semaphore: %v", err)
	}

	return nil
}

func (w *Worker) releaseRequestSemaphore() {
	w.maxConcurrentRequests.Release(1)
}

// callProcess will process a transaction and cancel it if we need to stop the
// worker.
func (w *Worker) callProcess(t transaction.Transaction) error {
	ctx := httptrace.WithClientTrace(w.workerCtx, transaction.GetClientTrace(w.log))

	// Block here if we are already sending too many requests
	err := w.acquireRequestSemaphore(ctx)
	if err != nil {
		w.requeue(t)
		return err
	}

	w.requestWg.Add(1)
	go func() {
		defer func() {
			w.requestWg.Done()
			w.releaseRequestSemaphore()
		}()
		w.process(ctx, t)
	}()

	return nil
}

func (w *Worker) process(ctx context.Context, t transaction.Transaction) {
	// Run the endpoint through our blockedEndpoints circuit breaker
	target := t.GetTarget()
	if w.blockedList.isBlockForSend(target, time.Now()) {
		w.requeue(t)
		w.log.Warnf("Too many errors for endpoint '%s': retrying later", target)
	} else if err := t.Process(ctx, w.config, w.log, w.Client.GetClient()); err != nil {
		w.blockedList.close(target, time.Now())
		w.requeue(t)
		w.log.Errorf("Error while processing transaction: %v", err)
	} else {
		w.pointSuccessfullySent.OnPointSuccessfullySent(t.GetPointCount())
		w.blockedList.recover(target, time.Now())
	}
}

func (w *Worker) requeue(t transaction.Transaction) {
	select {
	case w.RequeueChan <- t:
	default:
		w.log.Errorf("dropping transaction because the retry goroutine is too busy to handle another one")
	}
}
