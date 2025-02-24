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
	// DefaultWorkerCount By default most pipelines will only require a single sender worker, as the single worker itself can
	// concurrently transmit multiple http requests simultaneously. See the min/max worker concurrency settings.
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
	log.Infof("TEST: %s sender being constructed with min concurrency %d and max concurrency %d", componentName, minWorkerConcurrency, maxWorkerConcurrency)
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
	log.Infof("TEST: %s sender creating %d queues", componentName, len(queues))

	for i := range queuesCount {
		// create a queue
		queues[i] = make(chan *message.Payload, workersPerQueue+1)
		log.Infof("TEST: %s input created for pipeline %d", componentName, i)
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
		log.Infof("TEST: %s created %d senders for queue %d", componentName, workersPerQueue, i)
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
	log.Info("shared sender stopping")
	for _, s := range s.workers {
		s.stop()
	}
	for i := range s.queues {
		close(s.queues[i])
	}
}
