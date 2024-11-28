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
// TODO(remy): finish this work
type PipelineComponent interface {
	In() chan *message.Payload
	PipelineMonitor() metrics.PipelineMonitor
	Start()
	Stop()
}

// SharedSender distribute payloads on multiple
// underlying senders.
type SharedSender struct {
	senders []*Sender

	inputChan chan *message.Payload

	sharedInputChan chan *message.Payload
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
}

// NewSharedSender returns a new sender.
func NewSharedSender(sendersCount int,
	config pkgconfigmodel.Reader, inputChan chan *message.Payload, auditor auditor.Auditor, destinations *client.Destinations, bufferSize int,
	senderDoneChan chan *sync.WaitGroup, flushWg *sync.WaitGroup, pipelineMonitor metrics.PipelineMonitor) *SharedSender {
	var senders []*Sender

	sharedInputChan := make(chan *message.Payload, 20)
	for i := 0; i < sendersCount; i++ {
		sender := NewSender(config, sharedInputChan, auditor, destinations, bufferSize,
			senderDoneChan, flushWg, pipelineMonitor)
		senders = append(senders, sender)
	}

	log.Infof("created a shared sender with %d senders", len(senders))
	return &SharedSender{
		senders:         senders,
		pipelineMonitor: pipelineMonitor,
		utilization:     pipelineMonitor.MakeUtilizationMonitor("shared_sender"),
		inputChan:       inputChan,
		sharedInputChan: sharedInputChan,
	}
}

// In is the input channel of the shared sender
func (s *SharedSender) In() chan *message.Payload {
	return s.inputChan
}

// PipelineMonitor returns the pipeline monitor of the shared senders.
func (s *SharedSender) PipelineMonitor() metrics.PipelineMonitor {
	return s.pipelineMonitor
}

// Start starts all shared sender.
func (s *SharedSender) Start() {
	for _, sender := range s.senders {
		sender.Start()
	}
	go s.run()
}

func (s *SharedSender) run() {
	log.Info("shared sender starting")
	for payload := range s.inputChan {
		s.utilization.Start()
		s.sharedInputChan <- payload
		s.utilization.Stop()
	}
}

// Stop stops all shared senders.
func (s *SharedSender) Stop() {
	log.Info("shared sender stopping")
	for _, s := range s.senders {
		s.Stop()
	}
	close(s.sharedInputChan)
}
