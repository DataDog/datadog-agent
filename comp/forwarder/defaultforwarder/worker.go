// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarder

import (
	"context"
	"crypto/tls"
	"fmt"
	"golang.org/x/sync/semaphore"
	"net"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// Worker consumes Transaction (aka transactions) from the Forwarder and
// processes them. If the transaction fails to be processed the Worker will send
// it back to the Forwarder to be retried later.
type Worker struct {
	config config.Component
	log    log.Component

	// Client the http client used to processed transactions.
	Client *http.Client
	// HighPrio is the channel used to receive high priority transaction from the Forwarder.
	HighPrio <-chan transaction.Transaction
	// LowPrio is the channel used to receive low priority transaction from the Forwarder.
	LowPrio <-chan transaction.Transaction
	// RequeueChan is the channel used to send failed transaction back to the Forwarder.
	RequeueChan chan<- transaction.Transaction

	resetConnectionChan   chan struct{}
	stopChan              chan struct{}
	stopped               chan struct{}
	blockedList           *blockedEndpoints
	pointSuccessfullySent PointSuccessfullySent
	// If the client is for cluster agent
	isLocal bool

	// The maximum number of HTTP requests we can have inflight at any one time.
	maxConcurrentRequests *semaphore.Weighted
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
	isLocal bool,
	maxConcurrentRequests int64,
) *Worker {
	var maxConcurrentRequestsSem *semaphore.Weighted
	if maxConcurrentRequests > 0 {
		maxConcurrentRequestsSem = semaphore.NewWeighted(maxConcurrentRequests)
	}

	worker := &Worker{
		config:                config,
		log:                   log,
		HighPrio:              highPrioChan,
		LowPrio:               lowPrioChan,
		RequeueChan:           requeueChan,
		resetConnectionChan:   make(chan struct{}, 1),
		stopChan:              make(chan struct{}),
		stopped:               make(chan struct{}),
		Client:                NewHTTPClient(config),
		blockedList:           blocked,
		pointSuccessfullySent: pointSuccessfullySent,
		isLocal:               isLocal,
		maxConcurrentRequests: maxConcurrentRequestsSem,
	}
	if isLocal {
		worker.Client = newBearerAuthHTTPClient()
	}
	return worker
}

// NewHTTPClient creates a new http.Client
func NewHTTPClient(config config.Component) *http.Client {
	var transport *http.Transport

	transportConfig := config.Get("forwarder_http_protocol")

	switch transportConfig {
	case "http1":
		transport = httputils.CreateHTTPTransport(config, httputils.MaxConnsPerHost(1))
	case "auto":
		fallthrough
	default:
		transport = httputils.CreateHTTPTransport(config, httputils.WithHTTP2(), httputils.MaxConnsPerHost(1))
	}

	return &http.Client{
		Timeout:   config.GetDuration("forwarder_timeout") * time.Second,
		Transport: transport,
	}
}

func newBearerAuthHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   1 * time.Second,
				KeepAlive: 20 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     false,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
			TLSHandshakeTimeout:   5 * time.Second,
			MaxConnsPerHost:       1,
			MaxIdleConnsPerHost:   1,
			IdleConnTimeout:       60 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 3 * time.Second,
		},
		Timeout: 10 * time.Second,
	}
}

// Stop stops the worker.
func (w *Worker) Stop(purgeHighPrio bool) {
	w.stopChan <- struct{}{}
	<-w.stopped

	if purgeHighPrio {
		// purging waiting transactions
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

// ScheduleConnectionReset allows signaling the worker that all connections should
// be recreated before sending the next transaction. Returns immediately.
func (w *Worker) ScheduleConnectionReset() {
	select {
	case w.resetConnectionChan <- struct{}{}:
	default:
		// a reset is already planned, we can ignore this one
	}
}

// acquireRequestSemaphore attempts to acquire a semaphore, which will block
// if we are already sending too many requests.
// This can be bypassed by configuring `forwarder_max_concurrent_requests = 0`.
func (w *Worker) acquireRequestSemaphore(ctx context.Context) error {
	if w.maxConcurrentRequests != nil {
		err := w.maxConcurrentRequests.Acquire(ctx, 1)
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *Worker) releaseRequestSemaphore() {
	if w.maxConcurrentRequests != nil {
		w.maxConcurrentRequests.Release(1)
	}
}

// callProcess will process a transaction and cancel it if we need to stop the
// worker.
func (w *Worker) callProcess(t transaction.Transaction) error {
	// poll for connection reset events first
	select {
	case <-w.resetConnectionChan:
		w.resetConnections()
	default:
	}

	ctx, cancel := context.WithCancel(context.Background())
	ctx = httptrace.WithClientTrace(ctx, transaction.GetClientTrace(w.log))

	// Block here if we are already sending too many requests
	err := w.acquireRequestSemaphore(ctx)
	if err != nil {
		cancel()
		return err
	}

	done := make(chan interface{})
	go func() {
		defer cancel()
		w.process(ctx, t)
		w.releaseRequestSemaphore()
		done <- nil
	}()

	select {
	case <-w.stopChan:
		// cancel current Transaction if we need to stop the worker
		cancel()
		w.requeue(t)
		<-done // We still need to wait for the process func to return
		return fmt.Errorf("Worker was requested to stop")

	default:
		// Don't block
	}

	return nil
}

func (w *Worker) process(ctx context.Context, t transaction.Transaction) {
	// Run the endpoint through our blockedEndpoints circuit breaker
	target := t.GetTarget()
	if w.blockedList.isBlock(target) {
		w.requeue(t)
		w.log.Errorf("Too many errors for endpoint '%s': retrying later", target)
	} else if err := t.Process(ctx, w.config, w.log, w.Client); err != nil {
		w.blockedList.close(target)
		w.requeue(t)
		w.log.Errorf("Error while processing transaction: %v", err)
	} else {
		w.pointSuccessfullySent.OnPointSuccessfullySent(t.GetPointCount())
		w.blockedList.recover(target)
	}
}

func (w *Worker) requeue(t transaction.Transaction) {
	select {
	case w.RequeueChan <- t:
	default:
		w.log.Errorf("dropping transaction because the retry goroutine is too busy to handle another one")
	}
}

// resetConnections resets the connections by replacing the HTTP client used by
// the worker, in order to create new connections when the next transactions are processed.
// It must not be called while a transaction is being processed.
func (w *Worker) resetConnections() {
	w.log.Debug("Resetting worker's connections")
	w.Client.CloseIdleConnections()
	if w.isLocal {
		w.Client = newBearerAuthHTTPClient()
	} else {
		w.Client = NewHTTPClient(w.config)
	}
}
