// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SyncDestination sends a payload over HTTP and does not retry.
// In Serverless, the retry and backoff logic is handled by the serverless flush strategy
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
		destination:    newDestination(endpoint, contentType, destinationsContext, NoTimeoutOverride, false, destMeta, cfg, minConcurrency, maxConcurrency, metrics.NewNoopPipelineMonitor("0"), "0"),
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

	for p := range input {
		idle := float64(time.Since(startIdle) / time.Millisecond)
		d.destination.expVars.AddFloat(expVarIdleMsMapKey, idle)
		tlmIdle.Add(idle, d.destination.destMeta.TelemetryName())
		var startInUse = time.Now()

		err := d.destination.unconditionalSend(p)
		if err != nil {
			metrics.DestinationErrors.Add(1)
			metrics.TlmDestinationErrors.Inc()
			log.Debugf("Could not send payload: %v", err)
		}

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
