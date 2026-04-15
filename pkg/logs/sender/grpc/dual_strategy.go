// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	rtokenizer "github.com/DataDog/datadog-agent/pkg/logs/patterns/tokenizer/rust"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const grpcSecondaryBufferSize = 10000

// DualStrategy fans messages to both the primary HTTP strategy and a secondary gRPC stateful sender.
type DualStrategy struct {
	inputChan     chan *message.Message
	httpInputChan chan *message.Message
	grpcChan      chan *message.Message
	primary       sender.Strategy
	endpoint      config.Endpoint
	comp          compressioncommon.Compressor
	cfg           pkgconfigmodel.Reader
	endpoints     *config.Endpoints
	instanceID    string
	done          chan struct{}
}

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

	httpInputChan := make(chan *message.Message, cap(inputChan))
	primary := sender.NewBatchStrategy(
		httpInputChan,
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
		inputChan:     inputChan,
		httpInputChan: httpInputChan,
		grpcChan:      make(chan *message.Message, grpcSecondaryBufferSize),
		primary:       primary,
		endpoint:      grpcEndpoint,
		comp:          comp,
		cfg:           cfg,
		endpoints:     endpoints,
		instanceID:    instanceID,
		done:          make(chan struct{}),
	}
}

func (d *DualStrategy) Start() {
	d.primary.Start()

	conn, grpcClient, err := newGRPCClient(d.endpoint)
	if err != nil {
		log.Errorf("Failed to create gRPC connection for dual-send in pipeline %s: %v; continuing with HTTP only", d.instanceID, err)
		go func() {
			defer close(d.done)
			for msg := range d.inputChan {
				d.httpInputChan <- msg
			}
			close(d.httpInputChan)
		}()
		return
	}

	tokenizer := rtokenizer.NewRustTokenizer()
	translator := NewMessageTranslator(d.instanceID+"-dual", tokenizer)
	statefulChan := translator.Start(d.grpcChan, d.cfg.GetInt("logs_config.message_channel_size"))
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
		d.comp,
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
		d.endpoint,
		config.StreamLifetime(d.cfg),
		d.comp,
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
			d.httpInputChan <- msg
			d.grpcChan <- msg
		}

		close(d.httpInputChan)
		close(d.grpcChan)
	}()
}

func (d *DualStrategy) Stop() {
	<-d.done
	d.primary.Stop()
}
