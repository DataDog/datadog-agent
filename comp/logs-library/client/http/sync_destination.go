// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"context"
	"sync"
	"time"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// serverlessRetryBackoff is the wait between a serverless send and its single
// retry. It is kept small so the retry lands inside the minimum SIGTERM shutdown
// grace, where CPU is re-allocated.
const serverlessRetryBackoff = 250 * time.Millisecond

// SyncDestination sends a payload over HTTP synchronously. It is used in
// serverless, where each send must complete before the pipeline flush returns.
// On an idle CPU-throttled instance the first send can fail purely for lack of
// CPU, so a retryable failure is retried exactly once; any failure that survives
// the retry is dropped.
type SyncDestination struct {
	destination    *Destination
	senderDoneChan chan *sync.WaitGroup
}

// NewSyncDestination returns a new synchronous Destination.
func NewSyncDestination(
	endpoint config.Endpoint,
	contentType string,
	destinationsContext *client.DestinationsContext,
	senderDoneChan chan *sync.WaitGroup,
	destMeta *client.DestinationMetadata,
	cfg pkgconfigmodel.Reader,
) *SyncDestination {
	minConcurrency := 1
	maxConcurrency := minConcurrency

	return &SyncDestination{
		destination:    newDestination(endpoint, contentType, destinationsContext, NoTimeoutOverride, false, destMeta, cfg, minConcurrency, maxConcurrency, metrics.NewNoopPipelineMonitor("0"), "0", secretsnoopimpl.NewComponent().Comp),
		senderDoneChan: senderDoneChan,
	}
}

// IsMRF indicates that this destination is a Multi-Region Failover destination.
func (d *SyncDestination) IsMRF() bool {
	return d.destination.isMRF
}

// Target is the address of the destination.
func (d *SyncDestination) Target() string {
	return d.destination.url
}

// Metadata returns the metadata of the destination
func (d *SyncDestination) Metadata() *client.DestinationMetadata {
	return d.destination.destMeta
}

// Start starts reading the input channel
func (d *SyncDestination) Start(input chan *message.Payload, output chan *message.Payload, _ chan bool) (stopChan <-chan struct{}) {
	stop := make(chan struct{})
	go d.run(input, output, stop)
	return stop
}

func (d *SyncDestination) run(input chan *message.Payload, output chan *message.Payload, stopChan chan struct{}) {
	var startIdle = time.Now()
	ctx := d.destination.destinationsContext.Context()

	for p := range input {
		idle := float64(time.Since(startIdle) / time.Millisecond)
		d.destination.expVars.AddFloat(expVarIdleMsMapKey, idle)
		tlmIdle.Add(idle, d.destination.destMeta.TelemetryName())
		var startInUse = time.Now()

		d.sendWithRetry(ctx, p)

		if d.senderDoneChan != nil {
			// Notify the sender that the payload has been sent
			senderDoneWg := <-d.senderDoneChan
			senderDoneWg.Done()
		}

		metrics.LogsSent.Add(p.Count())
		metrics.TlmLogsSent.Add(float64(p.Count()))
		output <- p

		inUse := float64(time.Since(startInUse) / time.Millisecond)
		d.destination.expVars.AddFloat(expVarInUseMsMapKey, inUse)
		tlmInUse.Add(inUse, d.destination.destMeta.TelemetryName())
		startIdle = time.Now()
	}

	stopChan <- struct{}{}
}

// sendWithRetry sends the payload, retrying a retryable failure (network error,
// 5xx) exactly once after a short backoff. The single retry rides out the
// CPU-throttle window on an idle serverless instance and the retry lands
// once CPU is restored (next request or SIGTERM grace). A non-retryable failure
// or cancelled context or multiple failures drops the payload.
// Bounding the retry to one attempt keeps a persistently failing payload from
// starving the synchronous payloads behind it, while handling CPU-throttle.
func (d *SyncDestination) sendWithRetry(ctx context.Context, payload *message.Payload) {
	err := d.destination.unconditionalSend(payload)
	if err == nil {
		return
	}
	metrics.DestinationErrors.Add(1)
	metrics.TlmDestinationErrors.Inc()
	if _, retryable := err.(*client.RetryableError); !retryable {
		// Non-retryable (4xx) or context cancelled: drop.
		log.Debugf("Could not send payload: %v", err)
		return
	}

	log.Debugf("Could not send payload, retrying once CPU is available: %v", err)
	select {
	case <-ctx.Done():
		return
	case <-time.After(serverlessRetryBackoff):
	}

	if err := d.destination.unconditionalSend(payload); err != nil {
		metrics.DestinationErrors.Add(1)
		metrics.TlmDestinationErrors.Inc()
		log.Debugf("Could not send payload: %v", err)
	}
}
