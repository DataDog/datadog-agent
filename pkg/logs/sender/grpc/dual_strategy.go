// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/processor"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	httpClient "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	rtokenizer "github.com/DataDog/datadog-agent/pkg/logs/patterns/tokenizer/rust"
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const grpcSecondaryBufferSize = 10000

var tlmDualSendSecondaryDropped = telemetryimpl.GetCompatComponent().NewCounter(
	"logs_sender_grpc_dual_strategy",
	"secondary_dropped",
	[]string{"pipeline", "reason"},
	"Number of logs dropped from the secondary side of the gRPC dual-send strategy",
)

// DualStrategy fans messages to two transports simultaneously.
//
// HTTP-primary mode (NewDualStrategy): HTTP uses the pipeline's outputChan; gRPC is a
// standalone secondary with shadow headers injected.
//
// gRPC-primary mode (NewGRPCPrimaryDualStrategy): gRPC uses the pipeline's outputChan;
// HTTP additional endpoints are standalone secondaries. No shadow headers are injected.
type DualStrategy struct {
	inputChan     chan *message.Message
	primaryChan   chan *message.Message // feeds the primary strategy (outputChan side)
	secondaryChan chan *message.Message // feeds the standalone secondary
	primary       sender.Strategy
	grpcIsPrimary bool

	// HTTP-primary mode: standalone gRPC secondary
	grpcEndpoint config.Endpoint
	grpcComp     compressioncommon.Compressor

	// gRPC-primary mode: standalone HTTP secondaries
	httpEndpoints []config.Endpoint
	compression   logscompression.Component

	cfg             pkgconfigmodel.Reader
	endpoints       *config.Endpoints
	pipelineMonitor metrics.PipelineMonitor
	instanceID      string
	done            chan struct{}
}

// NewDualStrategy creates a DualStrategy in HTTP-primary mode.
// HTTP sends via the pipeline's outputChan; gRPC is a standalone secondary.
// Shadow headers (dd-shadow-only) are injected on the gRPC endpoint.
func NewDualStrategy(
	inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	grpcEndpoint config.Endpoint,
	comp compressioncommon.Compressor,
	cfg pkgconfigmodel.Reader,
	endpoints *config.Endpoints,
	serverlessMeta sender.ServerlessMeta,
	httpEncoder compressioncommon.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) sender.Strategy {
	if grpcEndpoint.ExtraHTTPHeaders == nil {
		grpcEndpoint.ExtraHTTPHeaders = map[string]string{}
	}
	grpcEndpoint.ExtraHTTPHeaders["dd-shadow-only"] = "true"

	httpPrimaryChan := make(chan *message.Message, cap(inputChan))
	httpPrimaryBatchStrategy := sender.NewBatchStrategy(
		httpPrimaryChan,
		outputChan,
		flushChan,
		serverlessMeta,
		endpoints.BatchWait,
		endpoints.BatchMaxSize,
		endpoints.BatchMaxContentSize,
		"logs",
		httpEncoder,
		pipelineMonitor,
		instanceID,
	)

	return &DualStrategy{
		inputChan:       inputChan,
		primaryChan:     httpPrimaryChan,
		secondaryChan:   make(chan *message.Message, grpcSecondaryBufferSize),
		primary:         httpPrimaryBatchStrategy,
		grpcIsPrimary:   false,
		grpcEndpoint:    grpcEndpoint,
		grpcComp:        comp,
		cfg:             cfg,
		endpoints:       endpoints,
		pipelineMonitor: pipelineMonitor,
		instanceID:      instanceID,
		done:            make(chan struct{}),
	}
}

// NewGRPCPrimaryDualStrategy creates a DualStrategy in gRPC-primary mode.
// gRPC sends via the pipeline's outputChan; HTTP additional endpoints are standalone secondaries.
// No shadow headers are injected on either path.
func NewGRPCPrimaryDualStrategy(
	inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	grpcComp compressioncommon.Compressor,
	cfg pkgconfigmodel.Reader,
	endpoints *config.Endpoints,
	httpEndpoints []config.Endpoint,
	compression logscompression.Component,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) sender.Strategy {
	primaryChan := make(chan *message.Message, grpcSecondaryBufferSize)

	// Build the gRPC primary strategy: primaryChan → translator → statefulChan → BatchStrategy → outputChan
	tokenizer := rtokenizer.NewRustTokenizer()
	translator := NewMessageTranslator(instanceID+"-grpc", tokenizer)
	statefulChan := translator.Start(primaryChan, cfg.GetInt("logs_config.message_channel_size"))
	primary := NewBatchStrategy(
		statefulChan,
		outputChan,
		flushChan,
		endpoints.BatchWait,
		endpoints.BatchMaxSize,
		endpoints.BatchMaxContentSize,
		"logs",
		grpcComp,
		pipelineMonitor,
		instanceID,
	)

	return &DualStrategy{
		inputChan:       inputChan,
		primaryChan:     primaryChan,
		secondaryChan:   make(chan *message.Message, grpcSecondaryBufferSize),
		primary:         primary,
		grpcIsPrimary:   true,
		httpEndpoints:   httpEndpoints,
		compression:     compression,
		cfg:             cfg,
		endpoints:       endpoints,
		pipelineMonitor: pipelineMonitor,
		instanceID:      instanceID,
		done:            make(chan struct{}),
	}
}

func (d *DualStrategy) Start() {
	d.primary.Start()

	if d.grpcIsPrimary {
		d.startHTTPSecondary()
	} else {
		d.startGRPCSecondary()
	}
}

// startGRPCSecondary handles the HTTP-primary mode: standalone gRPC as secondary.
func (d *DualStrategy) startGRPCSecondary() {
	conn, grpcClient, err := newGRPCClient(d.grpcEndpoint)
	if err != nil {
		log.Errorf("Failed to create gRPC connection for dual-send in pipeline %s: %v; continuing with HTTP only", d.instanceID, err)
		go func() {
			defer close(d.done)
			for msg := range d.inputChan {
				d.primaryChan <- msg
			}
			close(d.primaryChan)
		}()
		return
	}

	tokenizer := rtokenizer.NewRustTokenizer()
	translator := NewMessageTranslator(d.instanceID+"-dual", tokenizer)
	statefulChan := translator.Start(d.secondaryChan, d.cfg.GetInt("logs_config.message_channel_size"))
	payloadChan := make(chan *message.Payload, inputChanBufferSize)
	grpcFlushChan := make(chan struct{}, 1)

	pipelineMonitor := metrics.NewTelemetryPipelineMonitor()
	batchStrat := NewBatchStrategy(
		statefulChan,
		payloadChan,
		grpcFlushChan,
		d.endpoints.BatchWait,
		d.endpoints.BatchMaxSize,
		d.endpoints.BatchMaxContentSize,
		"grpc-dual",
		d.grpcComp,
		pipelineMonitor,
		d.instanceID,
	)
	batchStrat.Start()

	destCtx := client.NewDestinationsContext()
	destCtx.Start()

	maxInflight := d.cfg.GetInt("logs_config.grpc.max_inflight_payloads")
	if maxInflight <= 0 {
		maxInflight = pkgconfigsetup.DefaultMaxInflightPayloads
	}

	worker := newStreamWorker(
		"dual-"+d.instanceID,
		payloadChan,
		destCtx,
		conn,
		grpcClient,
		&sender.NoopSink{},
		d.grpcEndpoint,
		config.StreamLifetime(d.cfg),
		d.grpcComp,
		maxInflight,
	)
	worker.start()

	go func() {
		defer close(d.done)
		defer func() {
			batchStrat.Stop()
			worker.stop()
			destCtx.Stop()
			if err := conn.Close(); err != nil {
				log.Warnf("Error closing gRPC dual-send connection %s: %v", d.instanceID, err)
			}
		}()

		for msg := range d.inputChan {
			d.primaryChan <- msg   // HTTP primary
			d.secondaryChan <- msg // gRPC secondary
		}

		close(d.primaryChan)
		close(d.secondaryChan)
	}()
}

// startHTTPSecondary handles the gRPC-primary mode: standalone HTTP destinations as secondary.
// Messages are cloned and JSON-encoded before being fed into the existing sender.NewBatchStrategy,
// which handles batching and payload creation. No shadow headers are injected.
func (d *DualStrategy) startHTTPSecondary() {
	destCtx := client.NewDestinationsContext()
	destCtx.Start()

	httpAckSink := newNoopPayloadSink(100)

	// One input channel per HTTP destination for fan-out of payloads.
	destPayloadChans := make([]chan *message.Payload, len(d.httpEndpoints))
	httpStopChans := make([]<-chan struct{}, len(d.httpEndpoints))
	for i, ep := range d.httpEndpoints {
		ch := make(chan *message.Payload, 100)
		destPayloadChans[i] = ch
		destMeta := client.NewDestinationMetadata("logs", d.instanceID, "additional-http", ep.Host, "")
		dest := httpClient.NewDestination(ep, httpClient.JSONContentType, destCtx, true, destMeta, d.cfg, 1, 4, d.pipelineMonitor, d.instanceID, nil)
		httpStopChans[i] = dest.Start(ch, httpAckSink, nil)
	}

	// HTTP batch strategy: clonedChan → JSON payloads → httpOutputChan
	httpCloneInput := make(chan *message.Message, grpcSecondaryBufferSize)
	clonedChan := make(chan *message.Message, grpcSecondaryBufferSize)
	httpOutputChan := make(chan *message.Payload, 100)
	httpFlushChan := make(chan struct{}, 1)
	httpComp := d.compression.NewCompressor(compressioncommon.NoneKind, 0)
	httpBatch := sender.NewBatchStrategy(
		clonedChan,
		httpOutputChan,
		httpFlushChan,
		sender.NewServerlessMeta(false),
		d.endpoints.BatchWait,
		d.endpoints.BatchMaxSize,
		d.endpoints.BatchMaxContentSize,
		"logs-http-additional",
		httpComp,
		d.pipelineMonitor,
		d.instanceID,
	)
	httpBatch.Start()

	// Clone and encode HTTP-secondary messages off the primary fan-out goroutine.
	go d.encodeHTTPSecondary(httpCloneInput, clonedChan)

	// Fan httpOutputChan payloads to all HTTP destinations.
	go func() {
		for payload := range httpOutputChan {
			for _, ch := range destPayloadChans {
				ch <- payload
			}
		}
		for _, ch := range destPayloadChans {
			close(ch)
		}
	}()

	go func() {
		defer close(d.done)
		defer func() {
			httpBatch.Stop()
			for _, stopChan := range httpStopChans {
				if stopChan != nil {
					<-stopChan
				}
			}
			destCtx.Stop()
		}()

		for msg := range d.inputChan {
			d.primaryChan <- msg // gRPC primary (unmodified)

			select {
			case httpCloneInput <- msg:
			default:
				tlmDualSendSecondaryDropped.Inc(d.instanceID, "http_clone_queue_full")
			}
		}

		close(d.primaryChan)
		close(httpCloneInput)
	}()
}

func (d *DualStrategy) encodeHTTPSecondary(input <-chan *message.Message, output chan<- *message.Message) {
	defer close(output)
	for msg := range input {
		// Message embeds MessageContent by value so a copy is safe; JSONEncoder replaces
		// the Content slice rather than modifying in place.
		cloned := *msg
		if err := processor.JSONEncoder.Encode(&cloned, msg.MessageMetadata.Hostname); err != nil {
			log.Errorf("dual_strategy: failed to JSON-encode message for HTTP secondary: %v", err)
			tlmDualSendSecondaryDropped.Inc(d.instanceID, "http_encode_error")
			continue
		}
		output <- &cloned
	}
}

func newNoopPayloadSink(bufferSize int) chan *message.Payload {
	sink := make(chan *message.Payload, bufferSize)
	go func() {
		for payload := range sink {
			_ = payload
		}
	}()
	return sink
}

func (d *DualStrategy) Stop() {
	<-d.done
	d.primary.Stop()
}
