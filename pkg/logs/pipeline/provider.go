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
type Provider struct {
	numberOfPipelines int32
	chanSizes         int
	pipelinesChans    [](chan message.Message)

	currentChanIdx int32
}

// NewProvider returns a new Provider
func NewProvider() *Provider {
	return &Provider{
		numberOfPipelines: config.NumberOfPipelines,
		chanSizes:         config.ChanSizes,
		pipelinesChans:    [](chan message.Message){},
		currentChanIdx:    0,
	}
}

// Start initializes the pipelines
func (pp *Provider) Start(cm *sender.ConnectionManager, auditorChan chan message.Message) {

	for i := int32(0); i < pp.numberOfPipelines; i++ {

		senderChan := make(chan message.Message, pp.chanSizes)
		f := sender.New(senderChan, auditorChan, cm)
		f.Start()

		processorChan := make(chan message.Message, pp.chanSizes)
		p := processor.New(
			processorChan,
			senderChan,
			config.LogsAgent.GetString("api_key"),
			config.LogsAgent.GetString("logset"),
		)
		p.Start()

		pp.pipelinesChans = append(pp.pipelinesChans, processorChan)
	}
}

// MockPipelineChans initializes pipelinesChans for testing purpose
// TODO: move this somewhere else
func (pp *Provider) MockPipelineChans() {
	pp.pipelinesChans = [](chan message.Message){}
	pp.pipelinesChans = append(pp.pipelinesChans, make(chan message.Message))
	pp.numberOfPipelines = 1
}

// NextPipelineChan returns the next pipeline
func (pp *Provider) NextPipelineChan() chan message.Message {
	idx := atomic.AddInt32(&pp.currentChanIdx, 1)
	return pp.pipelinesChans[idx%pp.numberOfPipelines]
}
