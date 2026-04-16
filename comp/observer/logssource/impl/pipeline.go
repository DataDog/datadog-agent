// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

package logssourceimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
)

// observerPipeline is a pipeline.Provider that forwards log messages to the
// observer and discards them — no network sender.
type observerPipeline struct {
	proc       *processor.Processor
	inputChan  chan *message.Message
	outputChan chan *message.Message
	drainDone  chan struct{}
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
		processor.JSONEncoder,
		diagnostic.NewBufferedMessageReceiver(nil, hostname),
		hostname,
		pipelineMonitor,
		pipelineID,
		observerHandle,
	)
	return &observerPipeline{
		proc:       proc,
		inputChan:  inputChan,
		outputChan: outputChan,
		drainDone:  make(chan struct{}),
	}
}

// start starts the processor and the output drain goroutine.
// The drain goroutine MUST outlive proc.Stop() — see component.go OnStop for
// the required shutdown sequence.
func (p *observerPipeline) start() {
	go func() {
		defer close(p.drainDone)
		for msg := range p.outputChan {
			_ = msg
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
	return p.inputChan, nil
}

// GetOutputChan implements pipeline.Provider.
func (p *observerPipeline) GetOutputChan() chan *message.Message {
	return p.outputChan
}

// Start implements pipeline.Provider — component.go calls start() directly instead.
func (p *observerPipeline) Start() {}

// Stop implements pipeline.Provider — component.go handles ordered shutdown.
func (p *observerPipeline) Stop() {}

// Flush implements pipeline.Provider.
func (p *observerPipeline) Flush(ctx context.Context) {
	p.proc.Flush(ctx)
}
