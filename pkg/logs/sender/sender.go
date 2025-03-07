// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DefaultWorkerCount - By default most pipelines will only require a single sender worker, as the single worker itself can
	// concurrently transmit multiple http requests simultaneously. This value is not intended to be configurable, but legacy
	// usages of the sender will set this to their own defaults where necessary. See the BatchMaxConcurrentSends setting for
	// adjusting the sender concurrency values via config.
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

// NewSender returns a new sender.
func NewSender(
	config pkgconfigmodel.Reader,
	auditor auditor.Auditor,
	bufferSize int,
	senderDoneChan chan *sync.WaitGroup,
	flushWg *sync.WaitGroup,
	endpoints *config.Endpoints,
	destinationsCtx *client.DestinationsContext,
	status statusinterface.Status,
	serverless bool,
	componentName string,
	contentType string,
	workerCount int,
	minWorkerConcurrency int,
	maxWorkerConcurrency int,
) *Sender {
	log.Debugf(
		"Creating a new pipeline with %d sender workers, %d min sender concurrency, and %d max sender concurrency",
		workerCount,
		minWorkerConcurrency,
		maxWorkerConcurrency,
	)
	pipelineMonitor := metrics.NewTelemetryPipelineMonitor("sender_mux")

	var workers []*worker

	// It simplifies our workflows to keep the queues count value at one. Retaining the minimalistic logic required to support values larger than one
	// to allow us to easily explore alternate configurations moving forward.
	queuesCount := 1

	workersPerQueue := workerCount
	if workersPerQueue <= 0 {
		workersPerQueue = DefaultWorkerCount
	}

	queues := make([]chan *message.Payload, queuesCount)

	for i := range queuesCount {
		// create a queue
		queues[i] = make(chan *message.Payload, workersPerQueue+1)
		// output of this queue, create workers
		for range workersPerQueue {
			worker := newSenderWorker(
				config,
				auditor,
				bufferSize,
				senderDoneChan,
				flushWg,
				endpoints,
				destinationsCtx,
				status,
				serverless,
				componentName,
				contentType,
				minWorkerConcurrency,
				maxWorkerConcurrency,
				queues[i],
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

// In is the input channel of one or more sender workers
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
