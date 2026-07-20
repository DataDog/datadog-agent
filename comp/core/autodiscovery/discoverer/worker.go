// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"

	adtelemetry "github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	DefaultMaxAttempts = 5
	DefaultRetryDelay  = 10 * time.Second
	DefaultWorkerCount = 4
)

// jobKey uniquely identifies a discovery probe
type jobKey struct {
	svcID           string
	tplDigest       string
	integrationName string
}

// Config configures Worker construction
type Config struct {
	MaxAttempts int
	RetryDelay  time.Duration
	Workers     int
}

// Worker drives configuration-discovery probes asynchronously. It owns a
// delayed workqueue and a single goroutine that pops jobs, calls into the
// ConfigDiscoverer, and reports successful results back via the supplied
// ResultCallback.
type Worker struct {
	queue    workqueue.TypedDelayingInterface[jobKey]
	disco    ConfigDiscoverer
	services ServiceLookup
	onResult ResultCallback
	telStore *adtelemetry.Store

	maxAttempts int
	retryDelay  time.Duration
	workers     int

	m sync.Mutex
	// attempts tracks per-key failure counts so we can give up at maxAttempts
	attempts map[jobKey]int

	started     bool
	stopCh      chan struct{}
	workerWG    sync.WaitGroup
	startStopMu sync.Mutex
}

// NewWorker constructs a Worker. disco should never be nil.
func NewWorker(disco ConfigDiscoverer, services ServiceLookup, onResult ResultCallback, cfg Config, telStore *adtelemetry.Store) *Worker {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = DefaultMaxAttempts
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = DefaultRetryDelay
	}
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkerCount
	}
	return &Worker{
		queue: workqueue.NewTypedDelayingQueueWithConfig(workqueue.TypedDelayingQueueConfig[jobKey]{
			Name: "ADConfigDiscoverer",
		}),
		disco:       disco,
		services:    services,
		onResult:    onResult,
		maxAttempts: cfg.MaxAttempts,
		retryDelay:  cfg.RetryDelay,
		workers:     cfg.Workers,
		attempts:    map[jobKey]int{},
		telStore:    telStore,
	}
}

// Enqueue schedules a discovery probe for the given service / integration.
func (w *Worker) Enqueue(svcID, tplDigest, integrationName string) {
	k := jobKey{svcID: svcID, tplDigest: tplDigest, integrationName: integrationName}
	w.m.Lock()
	if _, alreadyPending := w.attempts[k]; !alreadyPending {
		w.attempts[k] = 0
		if w.telStore != nil {
			w.telStore.DiscoveryQueueDepth.Inc(integrationName, entityKindFromSvcID(svcID))
		}
	}
	w.m.Unlock()
	w.queue.Add(k)
}

// Start spins up the worker goroutines.
func (w *Worker) Start() {
	w.startStopMu.Lock()
	defer w.startStopMu.Unlock()
	if w.started {
		return
	}
	w.started = true
	w.stopCh = make(chan struct{})
	for range w.workers {
		w.workerWG.Add(1)
		go w.run()
	}
}

// Stop shuts the workqueue down and waits for all worker goroutines to exit.
func (w *Worker) Stop() {
	w.startStopMu.Lock()
	defer w.startStopMu.Unlock()
	if !w.started {
		return
	}
	w.started = false
	close(w.stopCh)
	w.queue.ShutDown()
	w.workerWG.Wait()
}

func (w *Worker) run() {
	defer w.workerWG.Done()
	wait.Until(func() {
		for w.processNext() {
		}
	}, time.Second, w.stopCh)
}

func (w *Worker) processNext() bool {
	key, quit := w.queue.Get()
	if quit {
		return false
	}
	defer w.queue.Done(key)

	// If service is not found, drop the attempt permanently.
	svc, ok := w.services.LookupService(key.svcID)
	if !ok {
		w.recordResult(key.integrationName, "service_not_found", key.svcID)
		w.dropAttempts(key)
		return true
	}

	serviceJSON, hasHost, err := marshalService(svc)
	if err != nil {
		log.Debugf("marshalling service %s failed: %v", key.svcID, err)
		w.requeueOrDrop(key)
		return true
	}
	if !hasHost {
		// No reachable host yet — typical during container startup. Retry.
		log.Debugf("service %s has no host yet, retrying", key.svcID)
		w.requeueOrDrop(key)
		return true
	}

	resultJSON, err := w.disco.DiscoverConfig(key.integrationName, serviceJSON)
	if err != nil {
		if _, ok := errors.AsType[PermFail](err); ok {
			log.Debugf("DiscoveryConfig for integration %s on service %s failed permanently: %v", key.integrationName, key.svcID, err)
			w.recordResult(key.integrationName, "permanent_failure", key.svcID)
			w.dropAttempts(key)
			return true
		}
		log.Debugf("DiscoveryConfig for integration %s on service %s failed: %v", key.integrationName, key.svcID, err)
		w.requeueOrDrop(key)
		return true
	}
	if resultJSON == "" {
		log.Debugf("DiscoveryConfig for integration %s returned empty result for %s", key.integrationName, key.svcID)
		w.requeueOrDrop(key)
		return true
	}

	configs, err := parseDiscoveryResult(key.integrationName, resultJSON)
	if err != nil {
		log.Warnf("DiscoveryConfig for integration %s returned invalid result for %s: %v", key.integrationName, key.svcID, err)
		w.requeueOrDrop(key)
		return true
	}
	if len(configs) == 0 {
		log.Debugf("DiscoveryConfig for integration %s returned no configs for %s", key.integrationName, key.svcID)
		w.requeueOrDrop(key)
		return true
	}

	w.recordResult(key.integrationName, "success", key.svcID)
	w.dropAttempts(key)
	w.onResult(key.svcID, key.tplDigest, configs)
	return true
}

func (w *Worker) requeueOrDrop(key jobKey) {
	w.m.Lock()
	if _, tracked := w.attempts[key]; tracked {
		w.attempts[key]++
	}
	attempt := w.attempts[key]
	w.m.Unlock()
	if attempt >= w.maxAttempts {
		log.Debugf("Giving up on DiscoveryConfig for integration %s for service %s after %d attempts",
			key.svcID, key.integrationName, attempt)
		w.recordResult(key.integrationName, "max_attempts_exceeded", key.svcID)
		w.dropAttempts(key)
		return
	}
	w.queue.AddAfter(key, w.retryDelay)
}

// dropAttempts releases the per-key retry counter on a terminal outcome.
func (w *Worker) dropAttempts(key jobKey) {
	w.m.Lock()
	if _, wasPending := w.attempts[key]; wasPending {
		delete(w.attempts, key)
		if w.telStore != nil {
			w.telStore.DiscoveryQueueDepth.Dec(key.integrationName, entityKindFromSvcID(key.svcID))
		}
	}
	w.m.Unlock()
}

// recordResult increments the discovery results counter for the given
// integration name, result type, and entity kind derived from svcID.
// It is a no-op when telemetry is not wired.
func (w *Worker) recordResult(integrationName, result, svcID string) {
	if w.telStore != nil {
		w.telStore.DiscoveryResults.Inc(integrationName, result, entityKindFromSvcID(svcID))
	}
}

// entityKindFromSvcID extracts the scheme prefix from a service ID of the form
// "<kind>://<id>" (e.g. "process", "docker", "containerd")
func entityKindFromSvcID(svcID string) string {
	if idx := strings.Index(svcID, "://"); idx != -1 {
		return svcID[:idx]
	}
	return "unknown"
}

// runOnce is exported for tests so they can drive the worker synchronously.
func (w *Worker) runOnce(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.processNext()
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
