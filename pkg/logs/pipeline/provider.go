// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"
	"hash/fnv"
	"strconv"
	"sync"

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

	failoverEnabled     bool
	sharedRouterChannel chan *message.Message
	forwarderWaitGroup  sync.WaitGroup
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

		failoverEnabled: cfg.GetBool("logs_config.pipeline_failover.enabled"),
	}
}

// Start initializes the pipelines and starts the sender.
// If failover is enabled (logs_config.pipeline_failover.enabled), it also creates
// a shared router channel and starts a forwarder goroutine that routes messages
// to pipelines with automatic failover when pipelines are blocked.
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

	if p.failoverEnabled {
		channelSize := p.cfg.GetInt("logs_config.message_channel_size")
		p.sharedRouterChannel = make(chan *message.Message, channelSize)
		p.forwarderWaitGroup.Add(1)
		go p.forwardWithFailover()
	}
}

// Stop stops all pipelines in parallel. This call blocks until all pipelines are stopped.
// If failover is enabled, it first closes the shared router channel and waits for the
// forwarder goroutine to finish processing any remaining messages before stopping pipelines.
func (p *provider) Stop() {
	stopper := startstop.NewParallelStopper()

	if p.sharedRouterChannel != nil {
		close(p.sharedRouterChannel)
	}
	p.forwarderWaitGroup.Wait()

	// close the pipelines
	for _, pipeline := range p.pipelines {
		stopper.Add(pipeline)
	}

	stopper.Stop()
	p.sender.Stop()
	p.pipelines = p.pipelines[:0]
}

// NextPipelineChan returns a channel for sending log messages to the pipeline.
// If failover is enabled, returns the shared router channel. All tailers share this
// channel, and messages are routed to pipelines based on origin hash for consistent
// primary pipeline affinity. If a pipeline is blocked, messages failover to other pipelines.
// If failover is disabled, returns a direct pipeline InputChan using round-robin selection.
func (p *provider) NextPipelineChan() chan *message.Message {
	pipelinesLen := len(p.pipelines)
	if pipelinesLen == 0 {
		return nil
	}

	if !p.failoverEnabled {
		// Legacy: direct pipeline access
		index := p.currentPipelineIndex.Inc() % uint32(pipelinesLen)
		nextPipeline := p.pipelines[index]
		return nextPipeline.InputChan
	}

	return p.sharedRouterChannel
}

func (p *provider) GetOutputChan() chan *message.Message {
	return nil
}

// NextPipelineChanWithMonitor returns a channel for sending log messages along with a capacity monitor.
// Used by file tailers which track capacity metrics.
// If failover is enabled, returns the shared router channel with a nil monitor. Ingress tracking
// is handled by the forwarder goroutine on the actual destination pipeline.
// If failover is disabled, returns a direct pipeline InputChan with its associated capacity monitor.
func (p *provider) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	pipelinesLen := len(p.pipelines)
	if pipelinesLen == 0 {
		return nil, nil
	}

	if !p.failoverEnabled {
		// Legacy behavior: direct pipeline access with pipeline monitor
		index := p.currentPipelineIndex.Inc() % uint32(pipelinesLen)
		nextPipeline := p.pipelines[index]
		return nextPipeline.InputChan, nextPipeline.pipelineMonitor.GetCapacityMonitor(metrics.ProcessorTlmName, strconv.Itoa(int(index)))
	}

	return p.sharedRouterChannel, nil
}

// forwardWithFailover reads messages from the shared router channel and routes them to pipelines.
// Each message's primary pipeline is determined by hashing its origin identifier, ensuring
// messages from the same source consistently go to the same pipeline (preserving order).
// If the primary pipeline is blocked (non-blocking send fails), it tries other pipelines.
// If all pipelines are blocked, it applies backpressure by blocking on the primary pipeline.
func (p *provider) forwardWithFailover() {
	defer p.forwarderWaitGroup.Done()

	for msg := range p.sharedRouterChannel {
		// Hash origin to get consistent primary pipeline
		primaryPipelineIndex := p.hashOriginToPipeline(msg.Origin)

		if !p.trySendToPipeline(msg, primaryPipelineIndex) {
			// All pipelines blocked, apply backpressure via blocking
			// and AddIngress on the primary pipeline
			p.pipelines[primaryPipelineIndex].InputChan <- msg // Blocks until pipeline accepts
			monitor := p.pipelines[primaryPipelineIndex].pipelineMonitor.GetCapacityMonitor(metrics.ProcessorTlmName, strconv.Itoa(int(primaryPipelineIndex)))
			monitor.AddIngress(msg)
		}
	}
}

// hashOriginToPipeline returns a consistent pipeline index for a given message origin.
// Uses FNV-1a hash of the most stable available identifier to ensure messages from the same source
// always route to the same primary pipeline. Falls back to round-robin if origin is nil.
func (p *provider) hashOriginToPipeline(origin *message.Origin) uint32 {
	pipelinesLen := uint32(len(p.pipelines))

	// Find the first stable identifier
	hashKey := p.getStableHashKey(origin)
	if hashKey == "" {
		return p.currentPipelineIndex.Inc() % pipelinesLen
	}

	pipelineHash := fnv.New32a()
	pipelineHash.Write([]byte(hashKey))
	return pipelineHash.Sum32() % uint32(len(p.pipelines))
}

// getStableHashKey returns the most stable identifier for an origin,
// Returns empty string if no stable identifer is available
func (p *provider) getStableHashKey(origin *message.Origin) string {
	if origin == nil {
		return ""
	}

	switch {
	case origin.Identifier != "":
		return origin.Identifier
	case origin.FilePath != "":
		return origin.FilePath
	case origin.LogSource != nil && origin.LogSource.Name != "":
		return origin.LogSource.Name
	default:
		return ""
	}
}

// trySendToPipeline attempts to send a message to a pipeline using non-blocking sends.
// It first tries the primary pipeline, then iterates through other pipelines if blocked.
// Returns true if the message was successfully sent, false if all pipelines are blocked.
// Tracks ingress metrics on the pipeline that actually receives the message.
func (p *provider) trySendToPipeline(msg *message.Message, primaryPipelineIndex uint32) bool {
	for attempt := 0; attempt < len(p.pipelines); attempt++ {
		idx := (primaryPipelineIndex + uint32(attempt)) % uint32(len(p.pipelines))
		pipeline := p.pipelines[idx]

		select {
		case pipeline.InputChan <- msg:
			// Ingress tracking on the pipeline
			monitor := pipeline.pipelineMonitor.GetCapacityMonitor(metrics.ProcessorTlmName, strconv.Itoa(int(idx)))
			monitor.AddIngress(msg)
			return true
		default:
			continue
		}
	}
	return false
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
