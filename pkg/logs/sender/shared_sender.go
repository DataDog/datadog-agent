// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"math"
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

	inputs []chan *message.Payload

	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor

	idx int
}

// NewSharedSender returns a new sender.
func NewSharedSender(sendersCount int,
	config pkgconfigmodel.Reader, auditor auditor.Auditor, destinations *client.Destinations, bufferSize int,
	senderDoneChan chan *sync.WaitGroup, flushWg *sync.WaitGroup, pipelineMonitor metrics.PipelineMonitor) *SharedSender {
	var senders []*Sender

	inputsCount := int(math.Ceil(float64(sendersCount) / 2))
	inputs := make([]chan *message.Payload, inputsCount)
	log.Infof("shared sender creating %d inputs", len(inputs))
	for i := 0; i < len(inputs); i++ {
		log.Info("input created")
		inputs[i] = make(chan *message.Payload, 2)
	}

	for i := 0; i < sendersCount; i++ {
		sender := NewSender(config, inputs[i%inputsCount], auditor, destinations, bufferSize,
			senderDoneChan, flushWg, pipelineMonitor)
		sender.isShared = true
		senders = append(senders, sender)
	}

	log.Infof("created a shared sender with %d senders and %d inputs", len(senders), len(inputs))
	return &SharedSender{
		senders:         senders,
		pipelineMonitor: pipelineMonitor,
		utilization:     pipelineMonitor.MakeUtilizationMonitor("shared_sender"),
		inputs:          inputs,
	}
}

// In is the input channel of the shared sender
func (s *SharedSender) In() chan *message.Payload {
	s.idx += 1
	return s.inputs[s.idx%len(s.inputs)]
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
}

// Stop stops all shared senders.
func (s *SharedSender) Stop() {
	log.Info("shared sender stopping")
	for _, s := range s.senders {
		s.Stop()
	}
	for i := range s.inputs {
		close(s.inputs[i])
	}
}
