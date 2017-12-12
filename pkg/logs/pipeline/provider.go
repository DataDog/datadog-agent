// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package pipeline

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// Provider provides message channels
type Provider interface {
	Start(cm *sender.ConnectionManager, auditorChan chan message.Message)
	NextPipelineChan() chan message.Message
}

// provider implements providing logic
type provider struct {
	numberOfPipelines int32
	chanSizes         int
	pipelinesChans    [](chan message.Message)

	currentChanIdx int32
}

// NewProvider returns a new Provider
func NewProvider() Provider {
	return &provider{
		numberOfPipelines: config.NumberOfPipelines,
		chanSizes:         config.ChanSizes,
		pipelinesChans:    [](chan message.Message){},
		currentChanIdx:    0,
	}
}

// Start initializes the pipelines
func (p *provider) Start(cm *sender.ConnectionManager, auditorChan chan message.Message) {

	for i := int32(0); i < p.numberOfPipelines; i++ {

		senderChan := make(chan message.Message, p.chanSizes)
		f := sender.New(senderChan, auditorChan, cm)
		f.Start()

		processorChan := make(chan message.Message, p.chanSizes)
		pr := processor.New(
			processorChan,
			senderChan,
			config.LogsAgent.GetString("api_key"),
			config.LogsAgent.GetString("logset"),
		)
		pr.Start()

		p.pipelinesChans = append(p.pipelinesChans, processorChan)
	}
}

// NextPipelineChan returns the next pipeline
func (p *provider) NextPipelineChan() chan message.Message {
	idx := atomic.AddInt32(&p.currentChanIdx, 1)
	return p.pipelinesChans[idx%p.numberOfPipelines]
}
