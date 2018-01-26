// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package pipeline

import (
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
)

// Provider provides message channels
type Provider interface {
	Start(cm *sender.ConnectionManager, auditorChan chan message.Message)
	Stop()
	NextPipelineChan() chan message.Message
}

// provider implements providing logic
type provider struct {
	chanSize             int
	pipelines            []*Pipeline
	currentPipelineIndex int32
	config               *config.Config
}

// NewProvider returns a new Provider
func NewProvider(config *config.Config) Provider {
	return &provider{
		chanSize:  config.GetChanSize(),
		pipelines: []*Pipeline{},
		config:    config,
	}
}

// Start initializes the pipelines
func (p *provider) Start(connManager *sender.ConnectionManager, outputChan chan message.Message) {
	for i := 0; i < p.config.GetNumberOfPipelines(); i++ {
		pipeline := NewPipeline(p.config, connManager, outputChan)
		pipeline.Start()
		p.pipelines = append(p.pipelines, pipeline)
	}
}

// Stop stops all the pipelines
func (p *provider) Stop() {
	wg := &sync.WaitGroup{}
	for _, pipeline := range p.pipelines {
		// stop all pipelines in parallel
		wg.Add(1)
		go func(p *Pipeline) {
			p.Stop()
			wg.Done()
		}(pipeline)
	}
	p.pipelines = p.pipelines[:0]
	wg.Wait()
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
