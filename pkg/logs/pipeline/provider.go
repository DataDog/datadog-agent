// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package pipeline

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// Provider provides message channels
type Provider interface {
	Start()
	Stop()
	NextPipelineChan() chan message.Message
}

// provider implements providing logic
type provider struct {
	connManager          *sender.ConnectionManager
	outputChan           chan message.Message
	chanSize             int
	pipelines            []*Pipeline
	currentPipelineIndex int32
	config               *config.Config
}

// NewProvider returns a new Provider
func NewProvider(config *config.Config, connManager *sender.ConnectionManager, outputChan chan message.Message) Provider {
	return &provider{
		connManager: connManager,
		outputChan:  outputChan,
		chanSize:    config.GetChanSize(),
		pipelines:   []*Pipeline{},
		config:      config,
	}
}

// Start initializes the pipelines
func (p *provider) Start() {
	for i := 0; i < p.config.GetNumberOfPipelines(); i++ {
		pipeline := NewPipeline(p.config, p.connManager, p.outputChan)
		pipeline.Start()
		p.pipelines = append(p.pipelines, pipeline)
	}
}

// Stop stops all pipelines in parallel,
// this call blocks until all pipelines are stopped
func (p *provider) Stop() {
	stopper := restart.NewParallelGroup()
	for _, pipeline := range p.pipelines {
		stopper.Add(pipeline)
	}
	stopper.Stop()
	p.pipelines = p.pipelines[:0]
}

// NextPipelineChan returns the next pipeline input channel
func (p *provider) NextPipelineChan() chan message.Message {
	pipelinesLen := len(p.pipelines)
	if pipelinesLen == 0 {
		return nil
	}
	index := atomic.AddInt32(&p.currentPipelineIndex, 1)
	nextPipeline := p.pipelines[int(index)%pipelinesLen]
	return nextPipeline.InputChan
}
