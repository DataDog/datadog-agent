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
)

// Provider provides message channels
type Provider interface {
	Start()
	Stop()
	NextPipelineChan() chan *message.Message
}

// provider implements providing logic
type provider struct {
	numberOfPipelines int
	outputChan        chan *message.Message
	endpoints         *config.Endpoints

	pipelines            []*Pipeline
	currentPipelineIndex int32
}

// NewProvider returns a new Provider
func NewProvider(numberOfPipelines int, outputChan chan *message.Message, endpoints *config.Endpoints) Provider {
	return &provider{
		numberOfPipelines: numberOfPipelines,
		outputChan:        outputChan,
		endpoints:         endpoints,
		pipelines:         []*Pipeline{},
	}
}

// Start initializes the pipelines
func (p *provider) Start() {
	for i := 0; i < p.numberOfPipelines; i++ {
		pipeline := NewPipeline(p.outputChan, p.endpoints)
		pipeline.Start()
		p.pipelines = append(p.pipelines, pipeline)
	}
}

// Stop stops all pipelines in parallel,
// this call blocks until all pipelines are stopped
func (p *provider) Stop() {
	stopper := restart.NewParallelStopper()
	for _, pipeline := range p.pipelines {
		stopper.Add(pipeline)
	}
	stopper.Stop()
	p.pipelines = p.pipelines[:0]
}

// NextPipelineChan returns the next pipeline input channel
func (p *provider) NextPipelineChan() chan *message.Message {
	pipelinesLen := len(p.pipelines)
	if pipelinesLen == 0 {
		return nil
	}
	index := int(p.currentPipelineIndex+1) % pipelinesLen
	defer atomic.StoreInt32(&p.currentPipelineIndex, int32(index))
	nextPipeline := p.pipelines[index]
	return nextPipeline.InputChan
}
