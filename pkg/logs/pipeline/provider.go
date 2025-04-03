// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// Provider provides message channels
type Provider interface {
	Start()
	Stop()
	NextPipelineChan() chan *message.Message
	GetOutputChan() chan *message.Message
	NextPipelineChanWithMonitor() (chan *message.Message, metrics.PipelineMonitor)
	// Flush flushes all pipeline contained in this Provider
	Flush(ctx context.Context)
}

// provider implements providing logic
type provider struct {
	numberOfPipelines         int
	auditor                   auditor.Auditor
	diagnosticMessageReceiver diagnostic.MessageReceiver
	outputChan                chan *message.Payload
	processingRules           []*config.ProcessingRule
	endpoints                 *config.Endpoints

	pipelines            []*Pipeline
	currentPipelineIndex *atomic.Uint32
	destinationsContext  *client.DestinationsContext

	serverless bool

	status      statusinterface.Status
	hostname    hostnameinterface.Component
	cfg         pkgconfigmodel.Reader
	compression logscompression.Component
}

// NewProvider returns a new Provider
func NewProvider(numberOfPipelines int,
	auditor auditor.Auditor,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	processingRules []*config.ProcessingRule,
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	status statusinterface.Status,
	hostname hostnameinterface.Component,
	cfg pkgconfigmodel.Reader,
	compression logscompression.Component,
) Provider {
	return newProvider(numberOfPipelines, auditor, diagnosticMessageReceiver, processingRules, endpoints, destinationsContext, false, status, hostname, cfg, compression)
}

// NewServerlessProvider returns a new Provider in serverless mode
func NewServerlessProvider(numberOfPipelines int,
	auditor auditor.Auditor,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	processingRules []*config.ProcessingRule,
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	status statusinterface.Status,
	hostname hostnameinterface.Component,
	cfg pkgconfigmodel.Reader,
	compression logscompression.Component,
) Provider {

	return newProvider(numberOfPipelines, auditor, diagnosticMessageReceiver, processingRules, endpoints, destinationsContext, true, status, hostname, cfg, compression)
}

// NewMockProvider creates a new provider that will not provide any pipelines.
func NewMockProvider() Provider {
	return &provider{}
}

func newProvider(numberOfPipelines int,
	auditor auditor.Auditor,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	processingRules []*config.ProcessingRule,
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	serverless bool,
	status statusinterface.Status,
	hostname hostnameinterface.Component,
	cfg pkgconfigmodel.Reader,
	compression logscompression.Component,
) Provider {
	return &provider{
		numberOfPipelines:         numberOfPipelines,
		auditor:                   auditor,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
		processingRules:           processingRules,
		endpoints:                 endpoints,
		pipelines:                 []*Pipeline{},
		currentPipelineIndex:      atomic.NewUint32(0),
		destinationsContext:       destinationsContext,
		serverless:                serverless,
		status:                    status,
		hostname:                  hostname,
		cfg:                       cfg,
		compression:               compression,
	}
}

// Start initializes the pipelines
func (p *provider) Start() {
	// This requires the auditor to be started before.
	p.outputChan = p.auditor.Channel()

	for i := 0; i < p.numberOfPipelines; i++ {
		pipeline := NewPipeline(p.outputChan, p.processingRules, p.endpoints, p.destinationsContext, p.diagnosticMessageReceiver, p.serverless, i, p.status, p.hostname, p.cfg, p.compression)
		pipeline.Start()
		p.pipelines = append(p.pipelines, pipeline)
	}
}

// Stop stops all pipelines in parallel,
// this call blocks until all pipelines are stopped
func (p *provider) Stop() {
	stopper := startstop.NewParallelStopper()
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
	index := p.currentPipelineIndex.Inc() % uint32(pipelinesLen)
	nextPipeline := p.pipelines[index]
	return nextPipeline.InputChan
}

func (p *provider) GetOutputChan() chan *message.Message {
	return nil
}

// NextPipelineChanWithMonitor returns the next pipeline input channel with it's monitor.
func (p *provider) NextPipelineChanWithMonitor() (chan *message.Message, metrics.PipelineMonitor) {
	pipelinesLen := len(p.pipelines)
	if pipelinesLen == 0 {
		return nil, nil
	}
	index := p.currentPipelineIndex.Inc() % uint32(pipelinesLen)
	nextPipeline := p.pipelines[index]
	return nextPipeline.InputChan, nextPipeline.pipelineMonitor
}

// Flush flushes synchronously all the contained pipeline of this provider.
func (p *provider) Flush(ctx context.Context) {
	for _, p := range p.pipelines {
		select {
		case <-ctx.Done():
			return
		default:
			p.Flush(ctx)
		}
	}
}
