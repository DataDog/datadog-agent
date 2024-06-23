// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
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
type SyncDestination struct {
	destination    *Destination
	senderDoneChan chan *sync.WaitGroup
}

// NewSyncDestination returns a new synchronous Destination.
func NewSyncDestination(endpoint config.Endpoint,
	contentType string,
	destinationsContext *client.DestinationsContext,
	senderDoneChan chan *sync.WaitGroup,
	telemetryName string,
	cfg pkgconfigmodel.Reader) *SyncDestination {

	return newSyncDestination(endpoint,
		contentType,
		destinationsContext,
		time.Second*10,
		senderDoneChan,
		telemetryName,
		cfg)
}

func newSyncDestination(endpoint config.Endpoint,
	contentType string,
	destinationsContext *client.DestinationsContext,
	timeout time.Duration,
	senderDoneChan chan *sync.WaitGroup,
	telemetryName string,
	cfg pkgconfigmodel.Reader) *SyncDestination {

	return &SyncDestination{
		destination:    newDestination(endpoint, contentType, destinationsContext, timeout, 1, false, telemetryName, cfg),
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

// Start starts reading the input channel
func (d *SyncDestination) Start(input chan *message.Payload, output chan *message.Payload, isRetrying chan bool) (stopChan <-chan struct{}) {
	stop := make(chan struct{})
	go d.run(input, output, stop)
	return stop
}

func (d *SyncDestination) run(input chan *message.Payload, output chan *message.Payload, stopChan chan struct{}) {
	var startIdle = time.Now()

	for p := range input {
		idle := float64(time.Since(startIdle) / time.Millisecond)
		d.destination.expVars.AddFloat(expVarIdleMsMapKey, idle)
		tlmIdle.Add(idle, d.destination.telemetryName)
		var startInUse = time.Now()

		err := d.destination.unconditionalSend(p)
		if err != nil {
			metrics.DestinationErrors.Add(1)
			metrics.TlmDestinationErrors.Inc()
			log.Debugf("Could not send payload: %v", err)
		}

		if d.senderDoneChan != nil {
			senderDoneWg := <-d.senderDoneChan
			senderDoneWg.Done()
		}

		metrics.LogsSent.Add(int64(len(p.Messages)))
		metrics.TlmLogsSent.Add(float64(len(p.Messages)))
		output <- p

		inUse := float64(time.Since(startInUse) / time.Millisecond)
		d.destination.expVars.AddFloat(expVarInUseMsMapKey, inUse)
		tlmInUse.Add(inUse, d.destination.telemetryName)
		startIdle = time.Now()
	}

	stopChan <- struct{}{}
}
