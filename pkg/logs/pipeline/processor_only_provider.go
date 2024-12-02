// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"context"
	"strconv"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sds"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// processorOnlyProvider implements the Provider provider interface and only contains the processor
type processorOnlyProvider struct {
	processor       *processor.Processor
	inputChan       chan *message.Message
	outputChan      chan *message.Message
	pipelineMonitor *metrics.TelemetryPipelineMonitor
}

// NewProcessorOnlyProvider is used by the logs check subcommand as the feature does not require the functionalities of the log pipeline other then the processor.
func NewProcessorOnlyProvider(diagnosticMessageReceiver diagnostic.MessageReceiver, processingRules []*config.ProcessingRule, cfg pkgconfigmodel.Reader, hostname hostnameinterface.Component) Provider {
	chanSize := 100
	outputChan := make(chan *message.Message, chanSize)
	encoder := processor.JSONServerlessEncoder
	inputChan := make(chan *message.Message, chanSize)
	pipelineID := 0
	pipelineMonitor := metrics.NewTelemetryPipelineMonitor(strconv.Itoa(pipelineID))
	processor := processor.New(cfg, inputChan, outputChan, processingRules,
		encoder, diagnosticMessageReceiver, hostname, pipelineMonitor)

	p := &processorOnlyProvider{
		processor:       processor,
		inputChan:       inputChan,
		outputChan:      outputChan,
		pipelineMonitor: pipelineMonitor,
	}

	return p
}

func (p *processorOnlyProvider) Start() {
	p.processor.Start()
}

func (p *processorOnlyProvider) Stop() {
	p.processor.Stop()
}

// return true if processor SDS scanners are active.
func (p *processorOnlyProvider) reconfigureSDS(config []byte, orderType sds.ReconfigureOrderType) (bool, error) {
	// Send a reconfiguration order to the running pipeline
	order := sds.ReconfigureOrder{
		Type:         orderType,
		Config:       config,
		ResponseChan: make(chan sds.ReconfigureResponse),
	}

	log.Debug("Sending SDS reconfiguration order:", string(order.Type))
	p.processor.ReconfigChan <- order

	// Receive response and determine if any errors occurred
	resp := <-order.ResponseChan
	scannerActive := resp.IsActive
	var rerr error
	if resp.Err != nil {
		rerr = multierror.Append(rerr, resp.Err)
	}

	return scannerActive, rerr
}

func (p *processorOnlyProvider) ReconfigureSDSStandardRules(standardRules []byte) (bool, error) {
	return p.reconfigureSDS(standardRules, sds.StandardRules)
}

// ReconfigureSDSAgentConfig reconfigures the pipeline with the given
// configuration received through Remote Configuration.
// Return true if all SDS scanners are active after applying this configuration.
func (p *processorOnlyProvider) ReconfigureSDSAgentConfig(config []byte) (bool, error) {
	return p.reconfigureSDS(config, sds.AgentConfig)
}

// StopSDSProcessing reconfigures the pipeline removing the SDS scanning
// from the processing steps.
func (p *processorOnlyProvider) StopSDSProcessing() error {
	_, err := p.reconfigureSDS(nil, sds.StopProcessing)
	return err
}

func (p *processorOnlyProvider) NextPipelineChan() chan *message.Message {
	return p.inputChan
}

func (p *processorOnlyProvider) NextPipelineChanWithMonitor() (chan *message.Message, metrics.PipelineMonitor) {
	return p.inputChan, p.pipelineMonitor
}

func (p *processorOnlyProvider) GetOutputChan() chan *message.Message {
	return p.outputChan
}

// Flush flushes synchronously all the contained pipeline of this provider.
func (p *processorOnlyProvider) Flush(ctx context.Context) {
	p.processor.Flush(ctx)
}
