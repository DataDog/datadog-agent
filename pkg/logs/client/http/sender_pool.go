// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http provides an HTTP destination for logs.
package http

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

const (
	ewmaAlpha                           = 0.064
	concurrentSendersEwmaSampleInterval = 1 * time.Second
)

// SenderPool is an interface for managing the concurrency of senders.
type SenderPool interface {
	// Perform an operation with the sender concurrency implementation.
	Run(func())
}

// limitedSenderPool is a senderPool implementation that limits the number of concurrent senders.
type limitedSenderPool struct {
	pool chan struct{}
}

// NewLimitedMaxSenderPool returns a new senderPool implementation that limits the number of concurrent senders.
func NewLimitedMaxSenderPool(maxWorkers int) SenderPool {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	return &limitedSenderPool{
		pool: make(chan struct{}, maxWorkers),
	}
}

func (l *limitedSenderPool) Run(doWork func()) {
	l.pool <- struct{}{}
	go func() {
		doWork()
		<-l.pool
	}()
}

type latencyThrottledSenderPool struct {
	pool                     chan struct{}
	virtualLatency           time.Duration
	inUseWorkers             int
	targetVirtualLatency     time.Duration
	maxWorkers               int
	virtualLatencyLastSample time.Time
	destMeta                 *client.DestinationMetadata
	windowSum                float64
	samples                  int
	ewmaSampleInterval       time.Duration
	alpha                    float64
}

// NewLatencyThrottledSenderPool returns a new senderPool implementation that limits the number of concurrent senders.
func NewLatencyThrottledSenderPool(maxWorkers int, targetLatency time.Duration, destMeta *client.DestinationMetadata) SenderPool {
	return newLatencyThrottledSenderPoolWithOptions(concurrentSendersEwmaSampleInterval, ewmaAlpha, maxWorkers, targetLatency, destMeta)
}

func newLatencyThrottledSenderPoolWithOptions(ewmaSampleInterval time.Duration, alpha float64, maxWorkers int, targetLatency time.Duration, destMeta *client.DestinationMetadata) *latencyThrottledSenderPool {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	sp := &latencyThrottledSenderPool{
		pool:                 make(chan struct{}, maxWorkers),
		maxWorkers:           maxWorkers,
		targetVirtualLatency: targetLatency,
		inUseWorkers:         1,
		destMeta:             destMeta,
		samples:              0,
		ewmaSampleInterval:   ewmaSampleInterval,
		alpha:                alpha,
	}
	// Start with 1 sender
	sp.pool <- struct{}{}
	return sp
}

func (l *latencyThrottledSenderPool) Run(doWork func()) {
	now := time.Now()
	<-l.pool
	go func() {
		doWork()
		l.pool <- struct{}{}
	}()
	l.resizePoolIfNeeded(now)
}

// Concurrency is scaled by attempting to converge on a target virtual latency.
// Virtual latency is the amount of time it takes to submit a payload to the worker pool.
// If Latency is above the target, more workers are added to the pool until the virtual latency is reached.
// This ensures the payload egress rate remains fair no matter how long the HTTP transaction takes
// (up to maxWorkers)
func (l *latencyThrottledSenderPool) resizePoolIfNeeded(then time.Time) {
	l.windowSum += float64(time.Since(then))
	l.samples++

	// Update the virtual latency every sample interval - an EWMA sampled every 1 second by default.
	if time.Since(l.virtualLatencyLastSample) >= l.ewmaSampleInterval && l.samples > 0 {
		avgLatency := l.windowSum / float64(l.samples)
		l.virtualLatency = time.Duration(float64(l.virtualLatency)*(1.0-ewmaAlpha) + (avgLatency * l.alpha))
		l.virtualLatencyLastSample = time.Now()
		l.windowSum = 0
		l.samples = 0
	}

	// If the virtual latency is above the target, add a worker to the pool.
	if l.virtualLatency > l.targetVirtualLatency && l.inUseWorkers < l.maxWorkers {
		l.pool <- struct{}{}
		l.inUseWorkers++
	} else if l.inUseWorkers > 1 {
		// else if the virtual latency is below the target, remove a worker from the pool if there is more than 1.
		<-l.pool
		l.inUseWorkers--
	}
	metrics.TlmNumSenders.Set(float64(l.inUseWorkers), l.destMeta.TelemetryName())
	metrics.TlmVirtualLatency.Set(float64(l.virtualLatency/time.Millisecond), l.destMeta.TelemetryName())
}
