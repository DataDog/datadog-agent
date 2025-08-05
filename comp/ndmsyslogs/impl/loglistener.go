package ndmsyslogsimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers/listener"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CustomTCPIntegration represents a custom TCP log integration using standard components
type CustomTCPIntegration struct {
	// Configuration
	port           int
	customTag      string
	customEndpoint string
	frameSize      int

	// Internal components
	source           *sources.LogSource
	tcpListener      *listener.TCPListener
	pipelineProvider pipeline.Provider

	// Pipeline components
	diagnosticMessageReceiver *diagnostic.BufferedMessageReceiver
	hostname                  hostnameinterface.Component
	cfg                       pkgconfigmodel.Reader
	compression               logscompression.Component
}

// NewCustomTCPIntegration creates a new custom TCP integration
func NewCustomTCPIntegration(
	port int,
	customTag,
	customEndpoint string,
	hostname hostnameinterface.Component,
	cfg pkgconfigmodel.Reader,
	compression logscompression.Component,
) *CustomTCPIntegration {
	return &CustomTCPIntegration{
		port:                      port,
		customTag:                 customTag,
		customEndpoint:            customEndpoint,
		frameSize:                 4096, // Default frame size
		diagnosticMessageReceiver: diagnostic.NewBufferedMessageReceiver(nil, nil),
		hostname:                  hostname,
		cfg:                       cfg,
		compression:               compression,
	}
}

// SetFrameSize sets the frame size for reading from connections
func (c *CustomTCPIntegration) SetFrameSize(frameSize int) {
	c.frameSize = frameSize
}

// Start starts the custom TCP integration
func (c *CustomTCPIntegration) Start() error {
	// Create log source with custom tag
	c.source = sources.NewLogSource("custom-tcp-integration", &config.LogsConfig{
		Type: config.TCPType,
		Port: c.port,
		Tags: []string{c.customTag},
	})

	// Create pipeline provider with custom destinations
	c.pipelineProvider = c.createPipelineProvider()

	// Create TCP listener directly
	c.tcpListener = listener.NewTCPListener(c.pipelineProvider, c.source, c.frameSize)

	// Start the pipeline provider
	c.pipelineProvider.Start()

	// Start the TCP listener
	c.tcpListener.Start()

	log.Infof("Started custom TCP integration on port %d with tag '%s'", c.port, c.customTag)
	return nil
}

// Stop stops the custom TCP integration
func (c *CustomTCPIntegration) Stop() {
	log.Infof("Stopping custom TCP integration on port %d", c.port)

	// Stop the TCP listener
	if c.tcpListener != nil {
		c.tcpListener.Stop()
	}

	// Stop pipeline provider
	if c.pipelineProvider != nil {
		c.pipelineProvider.Stop()
	}
}

// Flush flushes the pipeline
func (c *CustomTCPIntegration) Flush(ctx context.Context) {
	if c.pipelineProvider != nil {
		c.pipelineProvider.Flush(ctx)
	}
}

// createPipelineProvider creates a pipeline provider for the custom integration
func (c *CustomTCPIntegration) createPipelineProvider() pipeline.Provider {
	// Create destinations context
	destinationsContext := client.NewDestinationsContext()

	// Create processing rules (empty for now, but can be customized)
	processingRules := []*config.ProcessingRule{}

	// Create sink (noop sink since we handle destinations directly)
	sink := &sender.NoopSink{}

	// Create custom sender with custom destination factory
	senderImpl := c.createCustomSender(destinationsContext, sink)

	// Create pipeline provider with custom sender
	return c.createProviderWithCustomSender(
		processingRules,
		senderImpl,
	)
}

// createCustomSender creates a custom sender that sends to the custom endpoint
func (c *CustomTCPIntegration) createCustomSender(destinationsContext *client.DestinationsContext, sink sender.Sink) *sender.Sender {
	// Create custom destination factory
	destinationFactory := func(instanceID string) *client.Destinations {
		// Create custom endpoint
		endpoint := config.NewEndpoint("", "", c.customEndpoint, 443, "", true)

		// Create destination metadata
		destMeta := client.NewDestinationMetadata("custom-tcp-integration", instanceID, "http", c.customEndpoint)

		// Create HTTP destination
		destination := http.NewDestination(
			endpoint,
			http.JSONContentType,
			destinationsContext,
			true, // shouldRetry
			destMeta,
			c.cfg,
			1,  // minConcurrency
			10, // maxConcurrency
			metrics.NewNoopPipelineMonitor("custom-tcp-integration"),
			instanceID,
		)

		return client.NewDestinations([]client.Destination{destination}, nil)
	}

	// Create sender with custom destination factory
	return sender.NewSender(
		c.cfg,
		sink,
		destinationFactory,
		c.cfg.GetInt("logs_config.payload_channel_size"),
		sender.NewServerlessMeta(false),
		1, // queueCount
		1, // workersPerQueue
		metrics.NewNoopPipelineMonitor("custom-tcp-integration"),
	)
}

// createProviderWithCustomSender creates a provider with a custom sender
func (c *CustomTCPIntegration) createProviderWithCustomSender(
	processingRules []*config.ProcessingRule,
	senderImpl *sender.Sender,
) pipeline.Provider {
	// Create serverless meta
	serverlessMeta := sender.NewServerlessMeta(false)

	// Create provider using the pipeline package's internal function
	return c.createProvider(
		1, // numberOfPipelines
		c.diagnosticMessageReceiver,
		processingRules,
		serverlessMeta,
		senderImpl,
	)
}

// createProvider creates a provider with the given components
func (c *CustomTCPIntegration) createProvider(
	numberOfPipelines int,
	diagnosticMessageReceiver diagnostic.MessageReceiver,
	processingRules []*config.ProcessingRule,
	serverlessMeta sender.ServerlessMeta,
	senderImpl *sender.Sender,
) pipeline.Provider {
	// Create pipelines
	pipelines := make([]*pipeline.Pipeline, numberOfPipelines)
	for i := 0; i < numberOfPipelines; i++ {
		pipelines[i] = pipeline.NewPipeline(
			processingRules,
			nil, // endpoints not needed for custom pipeline
			senderImpl,
			c.diagnosticMessageReceiver,
			serverlessMeta,
			c.hostname,
			c.cfg,
			c.compression,
			"custom-tcp-integration",
		)
	}

	// Create provider
	return &customProvider{
		pipelines:                 pipelines,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
		sender:                    senderImpl,
		serverlessMeta:            serverlessMeta,
	}
}

// customProvider implements pipeline.Provider with custom sender
type customProvider struct {
	pipelines                 []*pipeline.Pipeline
	diagnosticMessageReceiver diagnostic.MessageReceiver
	sender                    *sender.Sender
	serverlessMeta            sender.ServerlessMeta
	currentPipelineIndex      int
}

func (p *customProvider) Start() {
	p.sender.Start()
	for _, pipeline := range p.pipelines {
		pipeline.Start()
	}
}

func (p *customProvider) Stop() {
	for _, pipeline := range p.pipelines {
		pipeline.Stop()
	}
	p.sender.Stop()
}

func (p *customProvider) NextPipelineChan() chan *message.Message {
	// Round-robin through pipelines
	pipeline := p.pipelines[p.currentPipelineIndex]
	p.currentPipelineIndex = (p.currentPipelineIndex + 1) % len(p.pipelines)
	return pipeline.InputChan
}

func (p *customProvider) GetOutputChan() chan *message.Message {
	// Return the first pipeline's output channel
	return p.pipelines[0].InputChan
}

func (p *customProvider) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	// Return the first pipeline's channel and a noop monitor
	return p.NextPipelineChan(), nil
}

func (p *customProvider) Flush(ctx context.Context) {
	for _, pipeline := range p.pipelines {
		pipeline.Flush(ctx)
	}
}

// GetPipelineProvider returns the pipeline provider for external use
func (c *CustomTCPIntegration) GetPipelineProvider() pipeline.Provider {
	return c.pipelineProvider
}

// GetMessageReceiver returns the diagnostic message receiver
func (c *CustomTCPIntegration) GetMessageReceiver() *diagnostic.BufferedMessageReceiver {
	return c.diagnosticMessageReceiver
}

// GetSource returns the log source
func (c *CustomTCPIntegration) GetSource() *sources.LogSource {
	return c.source
}
