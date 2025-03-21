// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"
	"sync"

	"github.com/hashicorp/go-multierror"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sds"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	httpsender "github.com/DataDog/datadog-agent/pkg/logs/sender/http"
	tcpsender "github.com/DataDog/datadog-agent/pkg/logs/sender/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	// maxConcurrencyPerPipeline is used to determine the maxSenderConcurrency value for the default provider creation logic.
	// We don't want to require users to know enough about our underlying architecture to understand what this value is meant
	// to do, so it's currently housed in a constant rather than a config entry. Users who wish to influence min/max
	// SenderConcurrency via config options should utilize the endpoint's BatchMaxConcurrentSend override instead.
	maxConcurrencyPerPipeline = 10
)

// Provider provides message channels
type Provider interface {
	Start()
	Stop()
	ReconfigureSDSStandardRules(standardRules []byte) (bool, error)
	ReconfigureSDSAgentConfig(config []byte) (bool, error)
	StopSDSProcessing() error
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
	sender                    sender.PipelineComponent

	pipelines            []*Pipeline
	currentPipelineIndex *atomic.Uint32
	destinationsContext  *client.DestinationsContext

	serverless bool
	flushWg    *sync.WaitGroup

	status      statusinterface.Status
	hostname    hostnameinterface.Component
	cfg         pkgconfigmodel.Reader
	compression logscompression.Component
}

// NewProvider returns a new Provider
func NewProvider(
	numberOfPipelines int,
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
	var senderDoneChan chan *sync.WaitGroup
	var flushWg *sync.WaitGroup
	serverless := false
	var workerCount, minSenderConcurrency, maxSenderConcurrency int
	if endpoints.UseHTTP {
		// If utililizing http, we can offload a large amount of concurrency to the http destination
		workerCount = sender.DefaultWorkerCount
		minSenderConcurrency = numberOfPipelines
		maxSenderConcurrency = numberOfPipelines * maxConcurrencyPerPipeline
		if endpoints.BatchMaxConcurrentSend != pkgconfigsetup.DefaultBatchMaxConcurrentSend {
			minSenderConcurrency = numberOfPipelines * endpoints.BatchMaxConcurrentSend
			maxSenderConcurrency = minSenderConcurrency
		}
	} else {
		// Currently the tcp destination is a synchronous entity. All concurrency needs to be in the form
		// of discrete sender workers.
		workerCount = numberOfPipelines
	}

	return newProvider(
		numberOfPipelines,
		auditor,
		diagnosticMessageReceiver,
		processingRules,
		endpoints,
		destinationsContext,
		senderDoneChan,
		flushWg,
		serverless,
		workerCount,
		status,
		hostname,
		cfg,
		compression,
		minSenderConcurrency,
		maxSenderConcurrency,
	)
}

// NewServerlessProvider Creates a pipeline setup that mirrors the legacy implementation of the senders.
// There will be one worker created for each pipeline, and each worker will only be able to run one
// operation at a time (unless overridden by DefaultBatchMaxConcurrentSend)
func NewServerlessProvider(
	numberOfPipelines int,
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
	senderDoneChan := make(chan *sync.WaitGroup)
	flushWg := &sync.WaitGroup{}
	serverless := true
	minSenderConcurrency := 1
	maxSenderConcurrency := minSenderConcurrency
	workerCount := numberOfPipelines

	return newProvider(
		numberOfPipelines,
		auditor,
		diagnosticMessageReceiver,
		processingRules,
		endpoints,
		destinationsContext,
		senderDoneChan,
		flushWg,
		serverless,
		workerCount,
		status,
		hostname,
		cfg,
		compression,
		minSenderConcurrency,
		maxSenderConcurrency,
	)
}

// NewMockProvider creates a new provider that will not provide any pipelines.
func NewMockProvider() Provider {
	return &provider{}
}

func newProvider(
	numberOfPipelines int,
	auditor auditor.Auditor,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	processingRules []*config.ProcessingRule,
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	senderDoneChan chan *sync.WaitGroup,
	flushWg *sync.WaitGroup,
	serverless bool,
	workerCount int,
	status statusinterface.Status,
	hostname hostnameinterface.Component,
	cfg pkgconfigmodel.Reader,
	compression logscompression.Component,
	minSenderConcurrency int,
	maxSenderConcurrency int,
) Provider {
	componentName := "logs"
	contentType := http.JSONContentType

	var senderImpl *sender.Sender

	if endpoints.UseHTTP {
		senderImpl = httpsender.NewHTTPSender(
			cfg,
			auditor,
			cfg.GetInt("logs_config.payload_channel_size"),
			senderDoneChan,
			flushWg,
			endpoints,
			destinationsContext,
			serverless,
			componentName,
			contentType,
			workerCount,
			minSenderConcurrency,
			maxSenderConcurrency,
		)
	} else {
		senderImpl = tcpsender.NewTCPSender(
			cfg,
			auditor,
			cfg.GetInt("logs_config.payload_channel_size"),
			senderDoneChan,
			flushWg,
			endpoints,
			destinationsContext,
			status,
			serverless,
			workerCount,
		)
	}

	return &provider{
		numberOfPipelines:         numberOfPipelines,
		auditor:                   auditor,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
		processingRules:           processingRules,
		endpoints:                 endpoints,
		sender:                    senderImpl,
		pipelines:                 []*Pipeline{},
		currentPipelineIndex:      atomic.NewUint32(0),
		destinationsContext:       destinationsContext,
		serverless:                serverless,
		flushWg:                   flushWg,
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
	p.sender.Start()

	for i := 0; i < p.numberOfPipelines; i++ {
		pipeline := NewPipeline(
			p.outputChan,
			p.processingRules,
			p.endpoints,
			p.destinationsContext,
			p.auditor,
			p.sender,
			p.diagnosticMessageReceiver,
			p.serverless,
			p.flushWg,
			i,
			p.status,
			p.hostname,
			p.cfg,
			p.compression,
		)
		pipeline.Start()
		p.pipelines = append(p.pipelines, pipeline)
	}
}

// Stop stops all pipelines in parallel,
// this call blocks until all pipelines are stopped
func (p *provider) Stop() {
	stopper := startstop.NewParallelStopper()

	// close the pipelines
	for _, pipeline := range p.pipelines {
		stopper.Add(pipeline)
	}

	stopper.Stop()
	p.sender.Stop()
	p.pipelines = p.pipelines[:0]
	p.outputChan = nil
}

// return true if all SDS scanners are active.
func (p *provider) reconfigureSDS(config []byte, orderType sds.ReconfigureOrderType) (bool, error) {
	var responses []chan sds.ReconfigureResponse

	// send a reconfiguration order to every running pipeline

	for _, pipeline := range p.pipelines {
		order := sds.ReconfigureOrder{
			Type:         orderType,
			Config:       config,
			ResponseChan: make(chan sds.ReconfigureResponse),
		}
		responses = append(responses, order.ResponseChan)

		log.Debug("Sending SDS reconfiguration order:", string(order.Type))
		pipeline.processor.ReconfigChan <- order
	}

	// reports if at least one error occurred

	var rerr error
	allScannersActive := true
	for _, response := range responses {
		resp := <-response

		if !resp.IsActive {
			allScannersActive = false
		}

		if resp.Err != nil {
			rerr = multierror.Append(rerr, resp.Err)
		}

		close(response)
	}

	return allScannersActive, rerr
}

// ReconfigureSDSStandardRules stores the SDS standard rules for the given provider.
func (p *provider) ReconfigureSDSStandardRules(standardRules []byte) (bool, error) {
	return p.reconfigureSDS(standardRules, sds.StandardRules)
}

// ReconfigureSDSAgentConfig reconfigures the pipeline with the given
// configuration received through Remote Configuration.
// Return true if all SDS scanners are active after applying this configuration.
func (p *provider) ReconfigureSDSAgentConfig(config []byte) (bool, error) {
	return p.reconfigureSDS(config, sds.AgentConfig)
}

// StopSDSProcessing reconfigures the pipeline removing the SDS scanning
// from the processing steps.
func (p *provider) StopSDSProcessing() error {
	_, err := p.reconfigureSDS(nil, sds.StopProcessing)
	return err
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
