// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sds"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// Provider provides message channels
type Provider interface {
	Start()
	Stop()
	ReconfigureSDSStandardRules(standardRules []byte) error
	ReconfigureSDSAgentConfig(config []byte) error
	NextPipelineChan() chan *message.Message
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

	status   statusinterface.Status
	hostname hostnameinterface.Component
	cfg      pkgconfigmodel.Reader
}

// NewProvider returns a new Provider
func NewProvider(numberOfPipelines int, auditor auditor.Auditor, diagnosticMessageReceiver diagnostic.MessageReceiver, processingRules []*config.ProcessingRule, endpoints *config.Endpoints, destinationsContext *client.DestinationsContext, status statusinterface.Status, hostname hostnameinterface.Component, cfg pkgconfigmodel.Reader) Provider {
	return newProvider(numberOfPipelines, auditor, diagnosticMessageReceiver, processingRules, endpoints, destinationsContext, false, status, hostname, cfg)
}

// NewServerlessProvider returns a new Provider in serverless mode
func NewServerlessProvider(numberOfPipelines int, auditor auditor.Auditor, processingRules []*config.ProcessingRule, endpoints *config.Endpoints, destinationsContext *client.DestinationsContext, status statusinterface.Status, hostname hostnameinterface.Component, cfg pkgconfigmodel.Reader) Provider {
	return newProvider(numberOfPipelines, auditor, &diagnostic.NoopMessageReceiver{}, processingRules, endpoints, destinationsContext, true, status, hostname, cfg)
}

// NewMockProvider creates a new provider that will not provide any pipelines.
func NewMockProvider() Provider {
	return &provider{}
}

func newProvider(numberOfPipelines int, auditor auditor.Auditor, diagnosticMessageReceiver diagnostic.MessageReceiver, processingRules []*config.ProcessingRule, endpoints *config.Endpoints, destinationsContext *client.DestinationsContext, serverless bool, status statusinterface.Status, hostname hostnameinterface.Component, cfg pkgconfigmodel.Reader) Provider {
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
	}
}

// Start initializes the pipelines
func (p *provider) Start() {
	// This requires the auditor to be started before.
	p.outputChan = p.auditor.Channel()

	for i := 0; i < p.numberOfPipelines; i++ {
		pipeline := NewPipeline(p.outputChan, p.processingRules, p.endpoints, p.destinationsContext, p.diagnosticMessageReceiver, p.serverless, i, p.status, p.hostname, p.cfg)
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

func (p *provider) reconfigureSDS(config []byte, orderType sds.ReconfigureOrderType) error {
	var responses []chan error

	// send a reconfiguration order to every running pipeline

	for _, pipeline := range p.pipelines {
		order := sds.ReconfigureOrder{
			Type:         orderType,
			Config:       config,
			ResponseChan: make(chan error),
		}
		responses = append(responses, order.ResponseChan)

		log.Debug("Sending SDS reconfiguration order:", string(order.Type))
		pipeline.processor.ReconfigChan <- order
	}

	// reports if at least one error occurred

	var rerr error
	for _, response := range responses {
		err := <-response
		if err != nil {
			rerr = multierror.Append(rerr, err)
		}
		close(response)
	}

	return rerr
}

// ReconfigureSDSStandardRules stores the SDS standard rules for the given provider.
func (p *provider) ReconfigureSDSStandardRules(standardRules []byte) error {
	return p.reconfigureSDS(standardRules, sds.StandardRules)
}

// ReconfigureSDSAgentConfig reconfigures the pipeline with the given
// configuration received through Remote Configuration.
func (p *provider) ReconfigureSDSAgentConfig(config []byte) error {
	return p.reconfigureSDS(config, sds.AgentConfig)
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
