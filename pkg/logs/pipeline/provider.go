// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

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
	pipelinesChans    [](chan message.Message)
	chanSize          int
	numberOfPipelines int32
	currentChanIdx    int32
	config            config.Config
}

// NewProvider returns a new Provider
func NewProvider(config *config.Config) Provider {
	return &provider{
		pipelinesChans:    [](chan message.Message){},
		chanSize:          config.GetChanSize(),
		numberOfPipelines: int32(config.GetNumberOfPipelines()),
		currentChanIdx:    0,
	}
}

// Start initializes the pipelines
func (p *provider) Start(cm *sender.ConnectionManager, auditorChan chan message.Message) {
	for i := int32(0); i < p.numberOfPipelines; i++ {

		senderChan := make(chan message.Message, p.chanSize)
		f := sender.New(senderChan, auditorChan, cm)
		f.Start()

		processorChan := make(chan message.Message, p.chanSize)
		pr := processor.New(
			processorChan,
			senderChan,
			p.config.GetAPIKey(),
			p.config.GetLogset(),
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
