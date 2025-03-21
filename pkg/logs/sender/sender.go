// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"sync"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DefaultWorkerCount - By default most pipelines will only require a single sender worker, as the single worker itself can
	// concurrently transmit multiple http requests at once. This value is not intended to be configurable, but legacy
	// usages of the sender will override this value where necessary. If there is a desire to edit the concurrency of the senders
	// via config, see the BatchMaxConcurrentSend endpoint setting.
	DefaultWorkerCount = 1
)

// PipelineComponent abstracts a pipeline component
type PipelineComponent interface {
	In() chan *message.Payload
	PipelineMonitor() metrics.PipelineMonitor
	Start()
	Stop()
}

// Sender can distribute payloads on multiple
// underlying workers
type Sender struct {
	workers []*worker
	queue   chan *message.Payload

	pipelineMonitor metrics.PipelineMonitor
	flushWg         *sync.WaitGroup
}

// DestinationFactory used to generate client destinations on each call.
type DestinationFactory func() *client.Destinations

// NewSender returns a new sender.
func NewSender(
	config pkgconfigmodel.Reader,
	auditor auditor.Auditor,
	destinationFactory DestinationFactory,
	bufferSize int,
	senderDoneChan chan *sync.WaitGroup,
	flushWg *sync.WaitGroup,
	workerCount int,
	pipelineMonitor metrics.PipelineMonitor,
) *Sender {
	var workers []*worker

	if workerCount <= 0 {
		workerCount = DefaultWorkerCount
	}

	queue := make(chan *message.Payload, workerCount+1)
	for range workerCount {
		worker := newWorker(
			config,
			queue,
			auditor,
			destinationFactory(),
			bufferSize,
			senderDoneChan,
			flushWg,
			pipelineMonitor,
		)
		workers = append(workers, worker)
	}

	return &Sender{
		workers:         workers,
		pipelineMonitor: pipelineMonitor,
		queue:           queue,
		flushWg:         flushWg,
	}
}

// In is the input channel of a worker set.
func (s *Sender) In() chan *message.Payload {
	return s.queue
}

// PipelineMonitor returns the pipeline monitor of the sender workers.
func (s *Sender) PipelineMonitor() metrics.PipelineMonitor {
	return s.pipelineMonitor
}

// Start starts all sender workers.
func (s *Sender) Start() {
	for _, worker := range s.workers {
		worker.start()
	}
}

// Stop stops all sender workers
func (s *Sender) Stop() {
	log.Info("sender mux stopping")
	for _, s := range s.workers {
		s.stop()
	}
	close(s.queue)
}
