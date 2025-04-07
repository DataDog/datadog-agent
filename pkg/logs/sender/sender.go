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

	"go.uber.org/atomic"
)

const (
	// DefaultWorkersPerQueue - By default most pipelines will only require a single sender worker, as the single worker itself can
	// concurrently transmit multiple http requests at once. This value is not intended to be configurable, but legacy
	// usages of the sender will override this value where necessary. If there is a desire to edit the concurrency of the senders
	// via config, see the BatchMaxConcurrentSend endpoint setting.
	DefaultWorkersPerQueue = 1

	// DefaultQueuesCount - By default most pipelines will only require a single queue, as the single queue itself can
	// concurrently transmit multiple http requests at once. Systems forced in to a legacy mode will override this value.
	DefaultQueuesCount = 1
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
	queues  []chan *message.Payload

	pipelineMonitor metrics.PipelineMonitor
	flushWg         *sync.WaitGroup
	idx             *atomic.Uint32
}

// DestinationFactory used to generate client destinations on each call.
type DestinationFactory func() *client.Destinations

// NewSender is the legacy sender.
func NewSender(
	config pkgconfigmodel.Reader,
	inputChan chan *message.Payload,
	outputChan chan *message.Payload,
	destinations *client.Destinations,
	bufferSize int,
	senderDoneChan chan *sync.WaitGroup,
	flushWg *sync.WaitGroup,
	pipelineMonitor metrics.PipelineMonitor,
) *Sender {
	w := newWorkerLegacy(
		config,
		inputChan,
		outputChan,
		destinations,
		bufferSize,
		senderDoneChan,
		flushWg,
		pipelineMonitor,
	)

	return &Sender{
		workers:         []*worker{w},
		pipelineMonitor: pipelineMonitor,
		queues:          []chan *message.Payload{inputChan},
		flushWg:         flushWg,
		idx:             &atomic.Uint32{},
	}
}

// NewSenderV2 returns a new sender.
func NewSenderV2(
	config pkgconfigmodel.Reader,
	auditor auditor.Auditor,
	destinationFactory DestinationFactory,
	bufferSize int,
	senderDoneChan chan *sync.WaitGroup,
	flushWg *sync.WaitGroup,
	queueCount int,
	workersPerQueue int,
	pipelineMonitor metrics.PipelineMonitor,
) *Sender {
	var workers []*worker

	if queueCount <= 0 {
		queueCount = DefaultQueuesCount
	}

	if workersPerQueue <= 0 {
		workersPerQueue = DefaultWorkersPerQueue
	}

	queues := make([]chan *message.Payload, queueCount)
	for idx := range queueCount {
		// Payloads are large, so the buffer will only hold one per worker
		queues[idx] = make(chan *message.Payload, workersPerQueue)
		for range workersPerQueue {
			worker := newWorker(
				config,
				queues[idx],
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
		idx:             &atomic.Uint32{},
	}
}

// In is the input channel of a worker set.
func (s *Sender) In() chan *message.Payload {
	idx := s.idx.Inc() % uint32(len(s.queues))
	return s.queues[idx]
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
	log.Debug("sender mux stopping")
	for _, s := range s.workers {
		s.stop()
	}
	for _, q := range s.queues {
		close(q)
	}
}
