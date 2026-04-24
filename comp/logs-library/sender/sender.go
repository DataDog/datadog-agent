// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"fmt"
	"sync"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
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

// Sink is the component that messages are sent to once the sender has finished processing them.
type Sink interface {
	Channel() chan *message.Payload
}

// NoopSink is a Sink implementation that does nothing
// This is used when there is no need to hook an auditor to the sender
type NoopSink struct{}

// Channel returns a nil channel
func (t *NoopSink) Channel() chan *message.Payload {
	return nil
}

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
	idx             *atomic.Uint32
}

// ServerlessMeta is a struct that contains essential control structures for serverless mode.
// Do not access any methods on this interface without checking IsEnabled first.
type ServerlessMeta interface {
	Lock()
	Unlock()
	WaitGroup() *sync.WaitGroup
	SenderDoneChan() chan *sync.WaitGroup
	IsEnabled() bool
}

// NewServerlessMeta creates a new ServerlessMeta instance.
func NewServerlessMeta(isEnabled bool) ServerlessMeta {
	if isEnabled {
		return &serverlessMetaImpl{sync.Mutex{}, sync.WaitGroup{}, make(chan *sync.WaitGroup), isEnabled}
	}
	return &serverlessMetaImpl{}
}

// serverlessMetaImpl is a struct that contains essential control structures for serverless mode.
type serverlessMetaImpl struct {
	sync.Mutex
	wg             sync.WaitGroup
	senderDoneChan chan *sync.WaitGroup
	enabled        bool
}

// WaitGroup returns the wait group for the serverless mode, used to block the pipeline flush until all payloads are sent.
func (s *serverlessMetaImpl) WaitGroup() *sync.WaitGroup {
	return &s.wg
}

// SenderDoneChan returns the channel is used to transfer wait groups from the sync_destination to the sender.
func (s *serverlessMetaImpl) SenderDoneChan() chan *sync.WaitGroup {
	return s.senderDoneChan
}

// IsEnabled returns true if the serverless mode is enabled.
// This is used to check if the serverless mode is enabled before accessing any of the methods on this struct.
func (s *serverlessMetaImpl) IsEnabled() bool {
	if s == nil {
		return false
	}
	return s.enabled
}

// DestinationFactory used to generate client destinations on each call.
type DestinationFactory func(id string) *client.Destinations

// NewSender returns a new sender.
func NewSender(
	config pkgconfigmodel.Reader,
	sink Sink,
	destinationFactory DestinationFactory,
	bufferSize int,
	serverlessMeta ServerlessMeta,
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
	for qidx := range queueCount {
		// Payloads are large, so the buffer will only hold one per worker
		queues[qidx] = make(chan *message.Payload, workersPerQueue)
		for widx := range workersPerQueue {
			workerID := fmt.Sprintf("q%ds%d", qidx, widx)
			worker := newWorker(
				config,
				queues[qidx],
				sink,
				destinationFactory,
				bufferSize,
				serverlessMeta,
				pipelineMonitor,
				workerID,
			)
			workers = append(workers, worker)
		}
	}

	return &Sender{
		workers:         workers,
		pipelineMonitor: pipelineMonitor,
		queues:          queues,
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
