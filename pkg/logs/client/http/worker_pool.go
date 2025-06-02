// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http provides an HTTP destination for logs.
package http

import (
	"math"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ewmaAlpha                           = 0.064
	concurrentSendersEwmaSampleInterval = 1 * time.Second
	targetLatency                       = 150 * time.Millisecond
)

// workerPool is an struct for managing the concurrency of destinations.
type workerPool struct {
	pool chan struct{}

	// virtualLatency is an ewma (exponentially weighted moving average) of the calculated latency seen by the pool
	virtualLatency time.Duration
	// ewmaSampleInterval is the interval over which the virtual latency is calculated.
	ewmaSampleInterval time.Duration
	// alpha is the alpha value for the ewma calculation.
	alpha float64

	// targetLatencyPerWorker is a runtime constant used to generate a target number of workers for a given virtual latency.
	targetLatencyPerWorker time.Duration

	inUseWorkers int

	// If minWorkers == maxWorkers, there are no latency calculations and no dynamic scaling.
	minWorkers int
	maxWorkers int

	destMeta *client.DestinationMetadata

	// windowSum is the sum of the latencies seen by the pool over the current ewmaSampleInterval.
	windowSum float64
	// samples is the number of samples taken over the current ewmaSampleInterval.
	samples int
	// shouldBackoff, if true, will reset the virtual latency to 0 and drop workers to the minWorkers count.
	shouldBackoff bool
	// virtualLatencyLastSample is the time of the last sample taken for the virtual latency calculation.
	virtualLatencyLastSample time.Time

	sync.Mutex
}

// newDefaultWorkerPool returns a new workerPool implementation that limits the number of concurrent destination jobs.
// It will supply production appropriate default values for fields such as targetLatency.
func newDefaultWorkerPool(minWorkers int, maxWorkers int, destMeta *client.DestinationMetadata) *workerPool {
	return newWorkerPool(
		concurrentSendersEwmaSampleInterval,
		ewmaAlpha,
		minWorkers,
		maxWorkers,
		targetLatency,
		destMeta,
	)
}

// newWorkerPool returns a new workerPool implementation that limits the number of concurrent destination jobs.
// targetLatency is divided by the minWorker count to generate the targetLatencyPerWorker value.
// Example: if the targetLatency is 150ms and the minWorkers is 4, the targetLatencyPerWorker will be 37.5ms
// If the virtualLatency is 300ms, the inUseWorkers will converge on 8.
// If the virtualLatency is 75ms, the inUseWorkers will converge on 4 (the value of minWorkers).
func newWorkerPool(
	ewmaSampleInterval time.Duration,
	alpha float64,
	minWorkers int,
	maxWorkers int,
	targetLatency time.Duration,
	destMeta *client.DestinationMetadata,
) *workerPool {
	if minWorkers <= 0 {
		minWorkers = 1
	}
	if maxWorkers < minWorkers {
		maxWorkers = minWorkers
	}
	targetLatencyPerWorker := targetLatency / time.Duration(minWorkers)

	sp := &workerPool{
		pool:                     make(chan struct{}, maxWorkers),
		minWorkers:               minWorkers,
		maxWorkers:               maxWorkers,
		targetLatencyPerWorker:   targetLatencyPerWorker,
		virtualLatencyLastSample: time.Now(),
		inUseWorkers:             minWorkers,
		destMeta:                 destMeta,
		samples:                  0,
		ewmaSampleInterval:       ewmaSampleInterval,
		alpha:                    alpha,
	}
	// Start with minWorker senders
	for range minWorkers {
		sp.pool <- struct{}{}
	}
	return sp
}

// run starts the doWork task in the pool. Will block if there are no
// workers available to pick up the task.
//
// This function is not concurrency safe and should not be called in a separate goroutine.
func (l *workerPool) run(doWork func() destinationResult) {
	l.resizeUnsafe()
	<-l.pool
	go func() {
		result := doWork()
		l.pool <- struct{}{}

		if l.maxWorkers == l.minWorkers {
			// If we can't change the worker count there's no point in adjusting latency calcs.
			return
		}
		l.Lock()
		defer l.Unlock()
		if result.err != nil {
			// We only want to trigger a backoff for retryable errors. Issues such as
			// server 400s should effectively freeze the pipeline pending a resolution.
			_, ok := result.err.(*client.RetryableError)
			l.shouldBackoff = l.shouldBackoff || ok
			return
		}

		l.windowSum += float64(result.latency)
		l.samples++
	}()
}

// Concurrency is scaled by attempting to converge on a target virtual latency.
// Virtual latency is the amount of time it takes to submit a payload to the worker pool.
// If Latency is above the target, more workers are added to the pool until the virtual latency is reached.
// This ensures the payload egress rate remains fair no matter how long the HTTP transaction takes
// (up to maxWorkers)
//
// This function is not concurrency safe and should not be called outside of the run function.
func (l *workerPool) resizeUnsafe() {
	if l.maxWorkers == l.minWorkers {
		// We can't resize, no variability in worker count allowed.
		return
	}

	if time.Since(l.virtualLatencyLastSample) >= l.ewmaSampleInterval {
		l.Lock()
		windowSum := l.windowSum
		samples := l.samples
		l.windowSum = 0
		l.samples = 0
		shouldBackoff := l.shouldBackoff
		l.shouldBackoff = false
		l.Unlock()

		if samples > 0 {
			avgLatency := windowSum / float64(samples)
			l.virtualLatency = time.Duration(float64(l.virtualLatency)*(1.0-ewmaAlpha) + (avgLatency * ewmaAlpha))
			l.virtualLatencyLastSample = time.Now()
		}

		targetWorkers := int(math.Ceil(float64(l.virtualLatency) / float64(l.targetLatencyPerWorker)))

		if shouldBackoff {
			metrics.TlmDestWorkerResets.Add(1, l.destMeta.TelemetryName())
			log.Debugf("Backing off sender pool workers in response to transient connection issues with destination.")
			// Backoff quickly on sender error
			l.virtualLatency = 0
			workersToDrop := l.inUseWorkers - l.minWorkers
			for i := 0; i < workersToDrop; i++ {
				<-l.pool
				l.inUseWorkers--
			}
		} else if targetWorkers > l.inUseWorkers && l.inUseWorkers < l.maxWorkers {
			// If the virtual latency is above the target, add a worker to the pool.
			l.pool <- struct{}{}
			l.inUseWorkers++
		} else if targetWorkers < l.inUseWorkers && l.inUseWorkers > l.minWorkers {
			// else if the virtual latency is below the target, remove a worker from the pool if there is more than minWorkers.
			<-l.pool
			l.inUseWorkers--
		}

		metrics.TlmDestNumWorkers.Set(float64(l.inUseWorkers), l.destMeta.TelemetryName())
		metrics.TlmDestVirtualLatency.Set(float64(l.virtualLatency/time.Millisecond), l.destMeta.TelemetryName())
		log.Debugf("Pool worker count at {%d} after resize, with virtual latency %s", l.inUseWorkers, l.virtualLatency)
	}
}
