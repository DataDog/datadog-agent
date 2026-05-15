// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	"context"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/processor"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// observerPipeline is a pipeline.Provider that forwards log messages to the
// observer without a network sender.
type observerPipeline struct {
	proc           *processor.Processor
	inputChan      chan *message.Message
	outputChan     chan *message.Message
	drainDone      chan struct{}
	observerHandle observer.Handle
}

func newObserverPipeline(
	cfg pkgconfigmodel.Reader,
	processingRules []*logsconfig.ProcessingRule,
	hostname hostnameinterface.Component,
	observerHandle observer.Handle,
) *observerPipeline {
	chanSize := cfg.GetInt("logs_config.message_channel_size")
	inputChan := make(chan *message.Message, chanSize)
	outputChan := make(chan *message.Message, chanSize)
	const pipelineID = "observer-logs-0"
	pipelineMonitor := metrics.NewNoopPipelineMonitor(pipelineID)
	proc := processor.New(
		cfg,
		inputChan,
		outputChan,
		processingRules,
		processor.PassthroughEncoder,
		diagnostic.NewBufferedMessageReceiver(nil, hostname),
		hostname,
		pipelineMonitor,
		pipelineID,
	)
	return &observerPipeline{
		proc:           proc,
		inputChan:      inputChan,
		outputChan:     outputChan,
		drainDone:      make(chan struct{}),
		observerHandle: observerHandle,
	}
}

// start starts the processor and the output drain goroutine.
// The drain goroutine MUST outlive proc.Stop() — see logssource.go OnStop for
// the required shutdown sequence.
func (p *observerPipeline) start() {
	go func() {
		defer close(p.drainDone)
		for msg := range p.outputChan {
			p.observerHandle.ObserveLog(&messageLogView{msg: msg})
		}
	}()
	p.proc.Start()
}

// NextPipelineChan implements pipeline.Provider.
func (p *observerPipeline) NextPipelineChan() chan *message.Message {
	return p.inputChan
}

// NextPipelineChanWithMonitor implements pipeline.Provider.
func (p *observerPipeline) NextPipelineChanWithMonitor() (chan *message.Message, *metrics.CapacityMonitor) {
	return p.inputChan, nil //nolint:nilnil
}

// GetOutputChan implements pipeline.Provider.
func (p *observerPipeline) GetOutputChan() chan *message.Message {
	return p.outputChan
}

// Start implements pipeline.Provider — logssource.go calls start() directly instead.
func (p *observerPipeline) Start() {}

// Stop implements pipeline.Provider — logssource.go handles ordered shutdown.
func (p *observerPipeline) Stop() {}

// Flush implements pipeline.Provider.
func (p *observerPipeline) Flush(ctx context.Context) {
	p.proc.Flush(ctx)
}

// messageLogView adapts *message.Message to observer.LogView.
// GetContent performs the single []byte→string conversion at the pipeline
// boundary; downstream extractors receive an immutable string with zero copies.
type messageLogView struct {
	msg *message.Message
}

func (v *messageLogView) GetContent() string           { return string(v.msg.GetContent()) }
func (v *messageLogView) GetStatus() string            { return v.msg.GetStatus() }
func (v *messageLogView) Tags() []string               { return v.msg.Tags() }
func (v *messageLogView) GetHostname() string          { return v.msg.GetHostname() }
func (v *messageLogView) GetTimestampUnixMilli() int64 { return v.msg.GetTimestampUnixMilli() }
