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

// syncRetryInitialBackoff and syncRetryMaxBackoff bound the wait between retries
// of a serverless send. The max is kept small so a retry attempt lands inside
// the ~10s SIGTERM shutdown grace, where CPU is re-allocated.
const (
	syncRetryInitialBackoff = 250 * time.Millisecond
	syncRetryMaxBackoff     = 2 * time.Second
)

// SyncDestination sends a payload over HTTP synchronously.
// On a retryable failure it retains the payload and keeps retrying rather than
// dropping it. A serverless (Cloud Run / Azure Container Apps) instance has its
// CPU throttled the instant a response returns, so a flush that fires while the
// instance is idle fails its POST only for lack of CPU. Because this destination
// does not release the serverless flush WaitGroup until the send completes (or
// the error is non-retryable, or shutdown cancels the context), the payload
// stays in flight and is delivered on the next window where CPU is available —
// the next request, or the SIGTERM grace at scale-to-zero. See SVLS-9268.
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

// sendWithRetry sends the payload, retrying retryable failures (network errors,
// 5xx) with capped backoff until the send succeeds or the destinations context
// is cancelled at shutdown. Non-retryable failures (4xx) and a cancelled context
// stop immediately, dropping the payload as before. While this is retrying, the
// caller has not released the serverless flush WaitGroup, so the pipeline flush
// waits for it — which is what lets a payload buffered on an idle, CPU-throttled
// instance reach Datadog once CPU is restored (next request, or scale-to-zero
// SIGTERM grace) instead of being dropped. See SVLS-9268.
func (d *SyncDestination) sendWithRetry(ctx context.Context, payload *message.Payload) {
	backoff := syncRetryInitialBackoff
	for {
		err := d.destination.unconditionalSend(payload)
		if err == nil {
			return
		}
		metrics.DestinationErrors.Add(1)
		metrics.TlmDestinationErrors.Inc()
		if _, retryable := err.(*client.RetryableError); !retryable {
			// Non-retryable (4xx) or context cancelled: drop, as before.
			log.Debugf("Could not send payload: %v", err)
			return
		}
		log.Debugf("Could not send payload, retrying once CPU is available: %v", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < syncRetryMaxBackoff {
			backoff *= 2
			if backoff > syncRetryMaxBackoff {
				backoff = syncRetryMaxBackoff
			}
		}
	}
}
