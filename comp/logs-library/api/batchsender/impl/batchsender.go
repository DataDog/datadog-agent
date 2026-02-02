// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package impl contains the implementation of the batch sender factory
package impl

import (
	"strconv"
	"strings"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logshttp "github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	httpsender "github.com/DataDog/datadog-agent/comp/logs-library/sender/http"
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"

	"github.com/DataDog/datadog-agent/comp/logs-library/api/batchsender/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/config"
	"github.com/DataDog/datadog-agent/comp/logs-library/message"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
)

// Dependencies are the dependencies for the batch sender factory
type Dependencies struct {
	Config     configcomp.Component
	Log        log.Component
	Compressor logscompression.Component
}

type Provides struct {
	Comp def.FactoryComponent
}

type pipeline struct {
	sender    sender.PipelineComponent
	strategy  sender.Strategy
	inputChan chan *message.Message
}

type batchSenderFactory struct {
	coreConfig configcomp.Component
	log        log.Component
	compressor logscompression.Component
}

func NewProvides(deps Dependencies) Provides {
	return Provides{
		Comp: NewBatchSenderFactory(deps.Config, deps.Log, deps.Compressor),
	}
}

// NewBatchSenderFactory creates a new batch sender factory
func NewBatchSenderFactory(config configcomp.Component, log log.Component, compressor logscompression.Component) def.FactoryComponent {
	return &batchSenderFactory{coreConfig: config, log: log, compressor: compressor}
}

// NewBatchSender creates a new batch sender pipeline
func (f *batchSenderFactory) NewBatchSender(
	endpoints *config.Endpoints,
	destinationsContext *client.DestinationsContext,
	eventType string,
	contentType string,
	category string,
	disableBatching bool,
	pipelineID int,
) def.BatchSender {
	pipelineMonitor := metrics.NewNoopPipelineMonitor(strconv.Itoa(pipelineID))

	inputChan := make(chan *message.Message, endpoints.InputChanSize)

	serverlessMeta := sender.NewServerlessMeta(false)
	senderImpl := httpsender.NewHTTPSender(
		f.coreConfig,
		&sender.NoopSink{},
		10, // Buffer Size
		serverlessMeta,
		endpoints,
		destinationsContext,
		eventType,
		contentType,
		category,
		sender.DefaultQueuesCount,
		sender.DefaultWorkersPerQueue,
		endpoints.BatchMaxConcurrentSend,
		endpoints.BatchMaxConcurrentSend,
	)

	var encoder compressioncommon.Compressor
	encoder = f.compressor.NewCompressor("none", 0)
	if endpoints.Main.UseCompression {
		encoder = f.compressor.NewCompressor(endpoints.Main.CompressionKind, endpoints.Main.CompressionLevel)
	}

	var strategy sender.Strategy

	if disableBatching || contentType == logshttp.ProtobufContentType {
		strategy = sender.NewStreamStrategy(inputChan, senderImpl.In(), encoder)
	} else {
		strategy = sender.NewBatchStrategy(
			inputChan,
			senderImpl.In(),
			make(chan struct{}),
			serverlessMeta,
			endpoints.BatchWait,
			endpoints.BatchMaxSize,
			endpoints.BatchMaxContentSize,
			eventType,
			encoder,
			pipelineMonitor,
			"0",
		)
	}

	f.log.Debugf("Initialized batch sender pipeline. eventType=%s mainHosts=%s additionalHosts=%s batch_max_concurrent_send=%d batch_max_content_size=%d batch_max_size=%d, input_chan_size=%d, compression_kind=%s, compression_level=%d",
		eventType,
		joinHosts(endpoints.GetReliableEndpoints()),
		joinHosts(endpoints.GetUnReliableEndpoints()),
		endpoints.BatchMaxConcurrentSend,
		endpoints.BatchMaxContentSize,
		endpoints.BatchMaxSize,
		endpoints.InputChanSize,
		endpoints.Main.CompressionKind,
		endpoints.Main.CompressionLevel,
	)

	return &pipeline{
		sender:    senderImpl,
		strategy:  strategy,
		inputChan: inputChan,
	}
}

// Start starts the batch sender pipeline
func (b *pipeline) Start() {
	b.sender.Start()
	b.strategy.Start()
}

// Stop stops the batch sender pipeline
func (b *pipeline) Stop() {
	b.strategy.Stop()
	b.sender.Stop()
}

// GetInputChan returns the input channel for the batch sender pipeline
func (b *pipeline) GetInputChan() chan *message.Message {
	return b.inputChan
}

func joinHosts(endpoints []config.Endpoint) string {
	var additionalHosts []string
	for _, e := range endpoints {
		additionalHosts = append(additionalHosts, e.Host)
	}
	return strings.Join(additionalHosts, ",")
}
