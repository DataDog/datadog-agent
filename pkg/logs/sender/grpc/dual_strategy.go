// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"google.golang.org/grpc"

	statefulpb "github.com/DataDog/agent-payload/v5/statefulpb"
	agentconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// grpcSecondaryBufferSize is the number of messages buffered for the gRPC secondary path.
const grpcSecondaryBufferSize = 10000

// DualStrategy implements sender.Strategy. It fans out each message to both a primary HTTP
// batch strategy and a secondary gRPC stateful sender. The gRPC path is fire-and-forget:
// if its buffer is full the message is dropped rather than blocking the primary path.
type DualStrategy struct {
	inputChan     chan *message.Message
	httpInputChan chan *message.Message
	grpcChan      chan *message.Message
	primary       sender.Strategy
	endpoint      agentconfig.Endpoint
	comp          compressioncommon.Compressor
	cfg           pkgconfigmodel.Reader
	endpoints     *agentconfig.Endpoints
	instanceID    string
	conn          *grpc.ClientConn
	grpcClient    statefulpb.StatefulLogsServiceClient
	done          chan struct{}
}

// NewDualStrategy returns a Strategy that fans messages to both an HTTP batch strategy
// and a secondary gRPC stateful sender. If the gRPC connection cannot be established it
// falls back to a plain HTTP batch strategy.
func NewDualStrategy(
	inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	grpcEndpoint agentconfig.Endpoint,
	comp compressioncommon.Compressor,
	cfg pkgconfigmodel.Reader,
	endpoints *agentconfig.Endpoints,
	serverlessMeta sender.ServerlessMeta,
	httpEncoder compressioncommon.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) sender.Strategy {
	grpcEndpoint.ExtraHeaders = map[string]string{"dd-shadow-only": "true"}
	conn, grpcClient, err := newGRPCClient(grpcEndpoint, cfg)
	if err != nil {
		log.Errorf("Failed to create gRPC connection for dual-send in pipeline %s: %v — falling back to HTTP only", instanceID, err)
		return sender.NewBatchStrategy(inputChan, outputChan, flushChan, serverlessMeta,
			endpoints.BatchWait, endpoints.BatchMaxSize, endpoints.BatchMaxContentSize,
			"logs", httpEncoder, pipelineMonitor, instanceID)
	}

	httpInputChan := make(chan *message.Message, cap(inputChan))
	primary := sender.NewBatchStrategy(httpInputChan, outputChan, flushChan, serverlessMeta,
		endpoints.BatchWait, endpoints.BatchMaxSize, endpoints.BatchMaxContentSize,
		"logs", httpEncoder, pipelineMonitor, instanceID)

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
		conn:          conn,
		grpcClient:    grpcClient,
		done:          make(chan struct{}),
	}
}

// Start starts the primary HTTP strategy and the secondary gRPC pipeline, then begins
// fanning out messages from inputChan to both.
func (d *DualStrategy) Start() {
	d.primary.Start()

	translator := NewMessageTranslator(d.instanceID + "-dual")
	statefulChan := make(chan *message.StatefulMessage, inputChanBufferSize)
	payloadChan := make(chan *message.Payload, inputChanBufferSize)
	flushChan := make(chan struct{}, 1)

	pipelineMonitor := metrics.NewTelemetryPipelineMonitor()
	batchStrat := NewBatchStrategy(
		statefulChan,
		payloadChan,
		flushChan,
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

	streamLifetime := agentconfig.StreamLifetime(d.cfg)
	worker := newStreamWorker(
		"dual-"+d.instanceID,
		payloadChan,
		destCtx,
		d.conn,
		d.grpcClient,
		&sender.NoopSink{},
		d.endpoint,
		streamLifetime,
		d.comp,
	)
	worker.start()

	// gRPC processor goroutine: reads from grpcChan and translates messages into the gRPC pipeline.
	grpcDone := make(chan struct{})
	go func() {
		defer close(grpcDone)
		for msg := range d.grpcChan {
			translator.processMessage(msg, statefulChan)
		}
		batchStrat.Stop()
		worker.stop()
		destCtx.Stop()
		if err := d.conn.Close(); err != nil {
			log.Warnf("Error closing gRPC dual-send connection %s: %v", d.instanceID, err)
		}
	}()

	// Fan-out goroutine: forwards each message to the primary HTTP strategy and the gRPC secondary.
	go func() {
		defer close(d.done)
		for msg := range d.inputChan {
			d.httpInputChan <- msg
			d.grpcChan <- msg
		}
		close(d.httpInputChan)
		close(d.grpcChan)
		<-grpcDone
	}()
}

// Stop waits for both pipelines to finish draining after inputChan is closed.
func (d *DualStrategy) Stop() {
	<-d.done
	d.primary.Stop()
}
