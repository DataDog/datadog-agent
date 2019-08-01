// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package pipeline

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
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
	auditor           *auditor.Auditor
	outputChan        chan *message.Message
	processingRules   []*config.ProcessingRule
	endpoints         *config.Endpoints

	pipelines            []*Pipeline
	currentPipelineIndex int32
	destinationsContext  *client.DestinationsContext
}

// NewProvider returns a new Provider
func NewProvider(numberOfPipelines int, auditor *auditor.Auditor, processingRules []*config.ProcessingRule, endpoints *config.Endpoints, destinationsContext *client.DestinationsContext) Provider {
	return &provider{
		numberOfPipelines:   numberOfPipelines,
		auditor:             auditor,
		processingRules:     processingRules,
		endpoints:           endpoints,
		pipelines:           []*Pipeline{},
		destinationsContext: destinationsContext,
	}
}

// Start initializes the pipelines
func (p *provider) Start() {
	// This requires the auditor to be started before.
	p.outputChan = p.auditor.Channel()

	for i := 0; i < p.numberOfPipelines; i++ {
		pipeline := NewPipeline(p.outputChan, p.processingRules, p.endpoints, p.destinationsContext)
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
	p.outputChan = nil
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
