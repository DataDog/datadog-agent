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

// SenderPool is an interface for managing the concurrency of senders.
type SenderPool interface {
	// Perform an operation with the sender concurrency implementation.
	Run(func() destinationResult)
}

type senderPool struct {
	pool                     chan struct{}
	virtualLatency           time.Duration
	shouldBackoff            bool
	inUseWorkers             int
	targetVirtualLatency     time.Duration
	minWorkers               int
	maxWorkers               int
	virtualLatencyLastSample time.Time
	destMeta                 *client.DestinationMetadata
	windowSum                float64
	samples                  int
	ewmaSampleInterval       time.Duration
	alpha                    float64
	sync.Mutex
}

// NewSenderPool returns a new senderPool implementation that limits the number of concurrent senders.
func NewSenderPool(minWorkers int, maxWorkers int, destMeta *client.DestinationMetadata) SenderPool {
	return newSenderPool(concurrentSendersEwmaSampleInterval, ewmaAlpha, minWorkers, maxWorkers, targetLatency/time.Duration(minWorkers), destMeta)
}

func newSenderPool(ewmaSampleInterval time.Duration, alpha float64, minWorkers int, maxWorkers int, targetLatencyPerWorker time.Duration, destMeta *client.DestinationMetadata) *senderPool {
	if minWorkers <= 0 {
		minWorkers = 1
	}
	if maxWorkers < minWorkers {
		maxWorkers = minWorkers
	}

	sp := &senderPool{
		pool:                     make(chan struct{}, maxWorkers),
		minWorkers:               minWorkers,
		maxWorkers:               maxWorkers,
		targetVirtualLatency:     targetLatencyPerWorker,
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

func (l *senderPool) Run(doWork func() destinationResult) {
	l.resize()
	<-l.pool
	go func() {
		result := doWork()
		l.pool <- struct{}{}

		if l.maxWorkers == l.minWorkers {
			return
		}
		l.Lock()
		defer l.Unlock()
		if result.err != nil {
			// We only want to trigger a backoff for retryable errors. Issues such as
			// server 500s should effectively freeze the pipeline pending a resolution.
			_, ok := result.err.(*client.RetryableError)
			l.shouldBackoff = l.shouldBackoff || ok
			return
		}
		log.Infof("TEST: Recording latency %d", result.latency)

		l.windowSum += float64(result.latency)
		l.samples++
	}()
}

// Concurrency is scaled by attempting to converge on a target virtual latency.
// Virtual latency is the amount of time it takes to submit a payload to the worker pool.
// If Latency is above the target, more workers are added to the pool until the virtual latency is reached.
// This ensures the payload egress rate remains fair no matter how long the HTTP transaction takes
// (up to maxWorkers)
func (l *senderPool) resize() {
	if l.maxWorkers == l.minWorkers {
		return
	}
	if time.Since(l.virtualLatencyLastSample) >= l.ewmaSampleInterval {
		l.Lock()
		log.Infof("TEST: Checking for resize: %d", l.samples)
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
			log.Infof("TEST: adjusted latency by %v", avgLatency)
			log.Infof("TEST: virtual latency = %v", l.virtualLatency)
		}

		targetWorkers := int(math.Ceil(float64(l.virtualLatency) / float64(l.targetVirtualLatency)))
		log.Infof("TEST: target workers = %d", targetWorkers)

		if shouldBackoff {
			// Backoff quickly on sender error
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

		metrics.TlmNumWorkers.Set(float64(l.inUseWorkers), l.destMeta.TelemetryName())
		metrics.TlmVirtualLatency.Set(float64(l.virtualLatency/time.Millisecond), l.destMeta.TelemetryName())
		log.Infof("TEST: worker count at {%d} after resize", l.inUseWorkers)
	}
}
