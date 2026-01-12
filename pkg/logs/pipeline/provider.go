// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"
	"strconv"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	httpsender "github.com/DataDog/datadog-agent/pkg/logs/sender/http"
	tcpsender "github.com/DataDog/datadog-agent/pkg/logs/sender/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	// maxConcurrencyPerPipeline is used to determine the maxSenderConcurrency value for the default provider creation logic.
	// We don't want to require users to know enough about our underlying architecture to understand what this value is meant
	// to do, so it's currently housed in a constant rather than a config entry. Users who wish to influence min/max
	// SenderConcurrency via config options should utilize the endpoint's BatchMaxConcurrentSend override instead.
	maxConcurrencyPerPipeline = 10

	// componentName is the name used for destination telemetry
	componentName = "logs"
)

var httpSenderFactory = httpsender.NewHTTPSender
var tcpSenderFactory = tcpsender.NewTCPSender

// Provider provides message channels
type Provider interface {
	Start()
	Stop()
	NextPipelineChan() chan *message.Message
	GetOutputChan() chan *message.Message
	NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor)
	// Flush flushes all pipeline contained in this Provider
	Flush(ctx context.Context)
}

// provider implements providing logic
type provider struct {
	numberOfPipelines         int
	diagnosticMessageReceiver diagnostic.MessageReceiver
	processingRules           []*config.ProcessingRule
	endpoints                 *config.Endpoints
	sender                    sender.PipelineComponent

	pipelines            []*Pipeline
	currentPipelineIndex *atomic.Uint32
	serverlessMeta       sender.ServerlessMeta

	hostname    hostnameinterface.Component
	cfg         pkgconfigmodel.Reader
	compression logscompression.Component
}

// NewProvider returns a new Provider
func NewProvider(
	numberOfPipelines int,
	sink sender.Sink,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	processingRules []*config.ProcessingRule,
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	status statusinterface.Status,
	hostname hostnameinterface.Component,
	cfg pkgconfigmodel.Reader,
	compression logscompression.Component,
	legacyMode bool,
	serverless bool,
) Provider {
	var senderImpl sender.PipelineComponent
	serverlessMeta := sender.NewServerlessMeta(serverless)

	if endpoints.UseHTTP {
		senderImpl = httpSender(numberOfPipelines, cfg, sink, endpoints, destinationsContext, serverlessMeta, legacyMode)
	} else {
		senderImpl = tcpSender(numberOfPipelines, cfg, sink, endpoints, destinationsContext, status, serverlessMeta, legacyMode)
	}

	return newProvider(
		numberOfPipelines,
		diagnosticMessageReceiver,
		processingRules,
		endpoints,
		hostname,
		cfg,
		compression,
		serverlessMeta,
		senderImpl,
	)
}

// NewMockProvider creates a new provider that will not provide any pipelines.
func NewMockProvider() Provider {
	return &provider{}
}

func tcpSender(
	numberOfPipelines int,
	cfg pkgconfigmodel.Reader,
	sink sender.Sink,
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	status statusinterface.Status,
	serverlessMeta sender.ServerlessMeta,
	legacy bool,
) *sender.Sender {
	var queueCount, workersPerQueue int
	if legacy {
		queueCount = numberOfPipelines
		workersPerQueue = 1
	} else {
		// Currently the tcp destination is a synchronous entity. All concurrency needs to be in the form
		// of discrete sender workers, so we spin up one per pipeline.
		queueCount = sender.DefaultQueuesCount
		workersPerQueue = numberOfPipelines
	}
	return tcpSenderFactory(
		cfg,
		sink,
		cfg.GetInt("logs_config.payload_channel_size"),
		serverlessMeta,
		endpoints,
		destinationsContext,
		status,
		componentName,
		queueCount,
		workersPerQueue,
	)
}

func httpSender(
	numberOfPipelines int,
	cfg pkgconfigmodel.Reader,
	sink sender.Sink,
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	serverlessMeta sender.ServerlessMeta,
	legacyMode bool,
) *sender.Sender {
	var queueCount, workersPerQueue, minSenderConcurrency, maxSenderConcurrency int
	if legacyMode {
		queueCount = numberOfPipelines
		workersPerQueue = 1
		minSenderConcurrency = endpoints.BatchMaxConcurrentSend
		maxSenderConcurrency = endpoints.BatchMaxConcurrentSend
	} else if serverlessMeta.IsEnabled() {
		// Currently the serverless destination is a synchronous entity. All concurrency needs to be in the form
		// of discrete sender workers, so we spin up one per pipeline.
		queueCount = sender.DefaultQueuesCount
		workersPerQueue = numberOfPipelines
		minSenderConcurrency = 1
		maxSenderConcurrency = 1
	} else {
		// If utililizing http, we can offload a large amount of concurrency to the http destination, while keeping queue and
		// worker counts low.
		queueCount = sender.DefaultQueuesCount
		workersPerQueue = sender.DefaultWorkersPerQueue
		minSenderConcurrency = numberOfPipelines
		maxSenderConcurrency = numberOfPipelines * maxConcurrencyPerPipeline
		if endpoints.BatchMaxConcurrentSend != pkgconfigsetup.DefaultBatchMaxConcurrentSend {
			// If the BatchMaxConcurrentSend parameter is set, we use it to control the concurrency of the destination.
			// Legacy behavior ran numberOfPipelines senders, each with a concurrency of BatchMaxConcurrentSend, so
			// we mimic that behavior here.
			minSenderConcurrency = numberOfPipelines * endpoints.BatchMaxConcurrentSend
			maxSenderConcurrency = minSenderConcurrency
		}
	}

	return httpSenderFactory(
		cfg,
		sink,
		cfg.GetInt("logs_config.payload_channel_size"),
		serverlessMeta,
		endpoints,
		destinationsContext,
		componentName,
		http.JSONContentType,
		"",
		queueCount,
		workersPerQueue,
		minSenderConcurrency,
		maxSenderConcurrency,
	)
}

func newProvider(
	numberOfPipelines int,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	processingRules []*config.ProcessingRule,
	endpoints *config.Endpoints,
	hostname hostnameinterface.Component,
	cfg pkgconfigmodel.Reader,
	compression logscompression.Component,
	serverlessMeta sender.ServerlessMeta,
	senderImpl sender.PipelineComponent,
) Provider {
	return &provider{
		numberOfPipelines:         numberOfPipelines,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
		processingRules:           processingRules,
		endpoints:                 endpoints,
		sender:                    senderImpl,
		pipelines:                 []*Pipeline{},
		currentPipelineIndex:      atomic.NewUint32(0),
		serverlessMeta:            serverlessMeta,
		hostname:                  hostname,
		cfg:                       cfg,
		compression:               compression,
	}
}

// Start initializes the pipelines
func (p *provider) Start() {
	p.sender.Start()

	for i := 0; i < p.numberOfPipelines; i++ {
		pipeline := NewPipeline(
			p.processingRules,
			p.endpoints,
			p.sender,
			p.diagnosticMessageReceiver,
			p.serverlessMeta,
			p.hostname,
			p.cfg,
			p.compression,
			strconv.Itoa(i),
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
func (p *provider) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	pipelinesLen := len(p.pipelines)
	if pipelinesLen == 0 {
		return nil, nil
	}
	index := p.currentPipelineIndex.Inc() % uint32(pipelinesLen)
	nextPipeline := p.pipelines[index]
	return nextPipeline.InputChan, nextPipeline.pipelineMonitor.GetCapacityMonitor(metrics.ProcessorTlmName, strconv.Itoa(int(index)))
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
	if p.serverlessMeta.IsEnabled() {
		p.serverlessMeta.Lock()
		defer p.serverlessMeta.Unlock()
		// Wait for the logs sender to finish sending payloads to all destinations before allowing the flush to finish
		p.serverlessMeta.WaitGroup().Wait()
	}
}
