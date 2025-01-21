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

// PipelineComponent abstracts a pipeline component
// TODO(remy): do not use "Component" naming and use this in more parts of the logs agent
type PipelineComponent interface {
	In() chan *message.Payload
	PipelineMonitor() metrics.PipelineMonitor
	Start()
	Stop()
}

// SharedSender distribute payloads on multiple
// underlying senders.
// Do not re-use a SharedSender, reinstantiate one instead.
type SharedSender struct {
	senders []*Sender
	started bool // can't be started twice
	stopped bool // but also can't be stopped twice
	mu      sync.Mutex

	queues []chan *message.Payload

	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor

	idx int
}

// NewSharedSender returns a new sender.
func NewSharedSender(config pkgconfigmodel.Reader, auditor auditor.Auditor, destinations *client.Destinations,
	bufferSize int, senderDoneChan chan *sync.WaitGroup, flushWg *sync.WaitGroup, pipelineMonitor metrics.PipelineMonitor) *SharedSender {
	var senders []*Sender

	queuesCount := config.GetInt("logs_config.queues_count")
	sendersPerQueue := config.GetInt("logs_config.senders_per_queue")

	queues := make([]chan *message.Payload, queuesCount)
	log.Infof("shared sender creating %d queues", len(queues))

	for i := 0; i < queuesCount; i++ {
		// create a queue
		queues[i] = make(chan *message.Payload, sendersPerQueue+1)
		log.Infof("input created for pipeline %d", i)
		// output of this queue, create senders
		for j := 0; j < sendersPerQueue; j++ {
			sender := NewSender(config, queues[i], auditor, destinations, bufferSize,
				senderDoneChan, flushWg, pipelineMonitor)
			sender.isShared = true
			senders = append(senders, sender)
		}
		log.Infof("created %d senders for queue %d", sendersPerQueue, i)
	}

	return &SharedSender{
		senders:         senders,
		pipelineMonitor: pipelineMonitor,
		utilization:     pipelineMonitor.MakeUtilizationMonitor("shared_sender"),
		queues:          queues,
	}
}

// In is the input channel of the shared sender
func (s *SharedSender) In() chan *message.Payload {
	s.idx++
	log.Infof("redistributed to input %d", s.idx%len(s.queues))
	return s.queues[s.idx%len(s.queues)]
}

// PipelineMonitor returns the pipeline monitor of the shared senders.
func (s *SharedSender) PipelineMonitor() metrics.PipelineMonitor {
	return s.pipelineMonitor
}

// Start starts all shared sender.
func (s *SharedSender) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return // do not start a shared sender twice
	}

	for _, sender := range s.senders {
		sender.Start()
	}

	s.started = true
}

// Stop stops all shared senders.
func (s *SharedSender) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return // do not stop a shared sender which has never been started
	}
	if s.stopped {
		return // do not stop a shared sender twice
	}

	log.Info("shared sender stopping")
	for _, s := range s.senders {
		s.Stop()
	}
	for i := range s.queues {
		close(s.queues[i])
	}

	s.stopped = true
}
