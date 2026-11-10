// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

// Worker consumes Transaction (aka transactions) from the Forwarder and
// processes them. If the transaction fails to be processed the Worker will send
// it back to the Forwarder to be retried later.
type Worker struct {
	config  config.Component
	log     log.Component
	secrets secrets.Component

	// Client the http client used to processed transactions.
	Client *SharedConnection
	// HighPrio is the channel used to receive high priority transaction from the Forwarder.
	HighPrio <-chan transaction.Transaction
	// LowPrio is the channel used to receive low priority transaction from the Forwarder.
	LowPrio <-chan transaction.Transaction
	// RequeueChan is the channel used to send failed transaction back to the Forwarder.
	RequeueChan chan<- transaction.Transaction

	stopped             chan struct{}
	stopChan            chan struct{}
	blockedList         *blockedEndpoints
	pointCountTelemetry PointCountTelemetry

	// waitForInflight controls Stop semantics. When true, Stop waits for all
	// in-flight HTTP requests to complete before returning. When false,
	// workerCtx is cancelled immediately on Stop, which aborts any in-flight
	// requests mid-transfer. Sourced from the forwarder_stop_wait_for_inflight
	// config key.
	waitForInflight bool

	// The maximum number of HTTP requests we can have inflight at any one time.
	maxConcurrentRequests weightedSemaphore
	requestCtx            context.Context
	workerCtx             context.Context
	cancel                context.CancelFunc
	requestWg             sync.WaitGroup
}

// PointCountTelemetry tracks the number of points that were either
// successfully delivered or dropped by the forwarder.
type PointCountTelemetry interface {
	OnPointSuccessfullySent(count int)
	OnPointDropped(count int)
}

// NewWorker returns a new worker to consume Transaction from inputChan
// and push back erroneous ones into requeueChan.
func NewWorker(
	config config.Component,
	log log.Component,
	secrets secrets.Component,
	highPrioChan <-chan transaction.Transaction,
	lowPrioChan <-chan transaction.Transaction,
	requeueChan chan<- transaction.Transaction,
	blocked *blockedEndpoints,
	pointCountTelemetry PointCountTelemetry,
	httpClient *SharedConnection,
	concurrency concurrencyDomain,
) *Worker {
	return newWorker(context.Background(), config, log, secrets, highPrioChan, lowPrioChan, requeueChan, blocked, pointCountTelemetry, httpClient, concurrency)
}

func newWorker(
	requestCtx context.Context,
	config config.Component,
	log log.Component,
	secrets secrets.Component,
	highPrioChan <-chan transaction.Transaction,
	lowPrioChan <-chan transaction.Transaction,
	requeueChan chan<- transaction.Transaction,
	blocked *blockedEndpoints,
	pointCountTelemetry PointCountTelemetry,
	httpClient *SharedConnection,
	concurrency concurrencyDomain,
) *Worker {
	workerCtx, cancel := context.WithCancel(requestCtx)

	worker := &Worker{
		config:                config,
		log:                   log,
		secrets:               secrets,
		HighPrio:              highPrioChan,
		LowPrio:               lowPrioChan,
		RequeueChan:           requeueChan,
		stopped:               make(chan struct{}),
		stopChan:              make(chan struct{}),
		Client:                httpClient,
		blockedList:           blocked,
		pointCountTelemetry:   pointCountTelemetry,
		waitForInflight:       config.GetBool("forwarder_stop_wait_for_inflight"),
		maxConcurrentRequests: concurrency.newRequestSemaphore(),
		requestCtx:            requestCtx,
		workerCtx:             workerCtx,
		cancel:                cancel,
	}
	return worker
}

// Stop stops the worker, dispatching to one of two implementations depending
// on w.waitForInflight (sourced from forwarder_stop_wait_for_inflight).
func (w *Worker) Stop(purgeHighPrio bool) {
	if w.waitForInflight {
		w.stopWaitingForInflight(purgeHighPrio)
	} else {
		w.stopWithoutWaitingForInflight(purgeHighPrio)
	}
}

// stopWithoutWaitingForInflight cancels workerCtx first so any transactions
// blocked on semaphore acquisition receive context.Canceled and are requeued,
// then waits for the Start goroutine to exit. If purgeHighPrio is set, a fresh
// workerCtx is installed so purge transactions can acquire the semaphore.
func (w *Worker) stopWithoutWaitingForInflight(purgeHighPrio bool) {
	w.cancel()
	close(w.stopChan)
	<-w.stopped
	if purgeHighPrio {
		w.drainHighPrioWithFreshContext()
	}
	w.requestWg.Wait()
	w.cancel()
}

// stopWaitingForInflight signals the Start goroutine via stopChan, allows
// in-flight HTTP requests to complete, and cancels workerCtx last.
// DefaultForwarder cancels requestCtx when its shutdown grace expires. Both
// in-flight HTTP requests and semaphore acquisition observe that cancellation.
// Stop still joins the worker loop and requests before their queues can close.
func (w *Worker) stopWaitingForInflight(purgeHighPrio bool) {
	close(w.stopChan)

	<-w.stopped

	if purgeHighPrio {
		w.drainHighPrioWithExistingContext()
	}

	w.requestWg.Wait()

	w.cancel()
}

// drainHighPrioWithExistingContext submits buffered high-priority work using
// the current workerCtx. requestWg owns completion of the HTTP processing.
func (w *Worker) drainHighPrioWithExistingContext() {
	w.drainHighPrio()
}

// drainHighPrioWithFreshContext replaces workerCtx with a fresh child
// context, then drains HighPrio. Used when the caller has already cancelled
// workerCtx and the drained transactions need a live context to acquire the
// semaphore.
func (w *Worker) drainHighPrioWithFreshContext() {
	w.workerCtx, w.cancel = context.WithCancel(w.requestCtx)
	w.drainHighPrio()
}

// drainHighPrio submits buffered transactions until HighPrio is empty or request
// admission is canceled. Remaining work stays with the domain owner.
// Callers should invoke one of the drainHighPrioWith* wrappers rather than
// this method directly.
func (w *Worker) drainHighPrio() {
	for {
		select {
		case t := <-w.HighPrio:
			w.log.Debugf("Flushing one new transaction before stopping Worker")
			if w.callProcess(t) != nil {
				// The current transaction was requeued. The domain owner retains
				// the remaining work for persistence or explicit discard.
				return
			}
		default:
			return
		}
	}
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
			case <-w.stopChan:
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
			case <-w.stopChan:
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
			w.releaseRequestSemaphore()
			w.requestWg.Done()
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
		return
	}
	if err := t.Process(ctx, w.config, w.log, w.secrets, w.Client.GetClient(), w.pointCountTelemetry); err != nil {
		if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
			// Local shutdown is not evidence that the intake endpoint failed.
			w.requeue(t)
			return
		}
		w.blockedList.close(target, time.Now())
		w.requeue(t)
		w.log.Errorf("Error while processing transaction: %v", err)
	} else {
		w.blockedList.recover(target, time.Now())
	}
}

func (w *Worker) requeue(t transaction.Transaction) {
	select {
	case w.RequeueChan <- t:
	default:
		w.pointCountTelemetry.OnPointDropped(t.GetPointCount())
		w.log.Errorf("dropping transaction because the retry goroutine is too busy to handle another one")
	}
}
