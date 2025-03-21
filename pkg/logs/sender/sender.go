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

// Sender distribute payloads on multiple
// underlying senders.
// Do not re-use a Sender, reinstantiate one instead.
type Sender struct {
	workers []*worker
	queues  []chan *message.Payload

	pipelineMonitor metrics.PipelineMonitor
	flushWg         *sync.WaitGroup

	idx int
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

	// Currently it simplifies our workflows to keep the queuesCount value at one. Retaining the minimalistic logic required to support values larger
	// than one to allow us to easily explore alternate configurations moving forward.
	queuesCount := 1

	workersPerQueue := workerCount
	if workersPerQueue <= 0 {
		workersPerQueue = DefaultWorkerCount
	}

	queues := make([]chan *message.Payload, queuesCount)

	for i := range queuesCount {
		queues[i] = make(chan *message.Payload, workersPerQueue+1)
		for range workersPerQueue {
			worker := newWorker(
				config,
				queues[i],
				auditor,
				destinationFactory(),
				bufferSize,
				senderDoneChan,
				flushWg,
				pipelineMonitor,
			)
			workers = append(workers, worker)
		}
	}

	return &Sender{
		workers:         workers,
		pipelineMonitor: pipelineMonitor,
		queues:          queues,
		flushWg:         flushWg,
	}
}

// In is the input channel of a worker.
func (s *Sender) In() chan *message.Payload {
	s.idx = (s.idx + 1) % len(s.queues)
	log.Infof("redistributed to input %d", s.idx)
	return s.queues[s.idx]
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
	for i := range s.queues {
		close(s.queues[i])
	}
}
