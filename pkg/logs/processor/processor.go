// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"context"
	"time"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sds"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// UnstructuredProcessingMetricName collects how many rules are used on unstructured
// content for tailers capable of processing both unstructured and structured content.
const UnstructuredProcessingMetricName = "datadog.logs_agent.tailer.unstructured_processing"

// A Processor updates messages from an inputChan and pushes
// in an outputChan.
type Processor struct {
	inputChan  chan *message.Message
	outputChan chan *message.Message // strategy input
	// ReconfigChan transports rules to use in order to reconfigure
	// the processing rules of the SDS Scanner.
	ReconfigChan              chan sds.ReconfigureOrder
	processingRules           []*config.ProcessingRule
	encoder                   Encoder
	done                      chan struct{}
	diagnosticMessageReceiver diagnostic.MessageReceiver
	flareCtl                  *flare.FlareController
	mu                        sync.Mutex
	hostname                  hostnameinterface.Component

	sds *sds.Scanner // configured through RC
}

// New returns an initialized Processor.
func New(inputChan, outputChan chan *message.Message, processingRules []*config.ProcessingRule, encoder Encoder,
	diagnosticMessageReceiver diagnostic.MessageReceiver, hostname hostnameinterface.Component,
	flareCtl *flare.FlareController) *Processor {
	sdsScanner := sds.CreateScanner()

	return &Processor{
		inputChan:                 inputChan,
		outputChan:                outputChan, // strategy input
		ReconfigChan:              make(chan sds.ReconfigureOrder),
		processingRules:           processingRules,
		encoder:                   encoder,
		done:                      make(chan struct{}),
		sds:                       sdsScanner,
		flareCtl:                  flareCtl,
		diagnosticMessageReceiver: diagnosticMessageReceiver,
		hostname:                  hostname,
	}
}

// Start starts the Processor.
func (p *Processor) Start() {
	go p.run()
}

// Stop stops the Processor,
// this call blocks until inputChan is flushed
func (p *Processor) Stop() {
	if p.sds != nil {
		p.sds.Delete()
		p.sds = nil
	}
	close(p.inputChan)
	<-p.done
}

// Flush processes synchronously the messages that this processor has to process.
func (p *Processor) Flush(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if len(p.inputChan) == 0 {
				return
			}
			msg := <-p.inputChan
			p.processMessage(msg)
		}
	}
}

// run starts the processing of the inputChan
func (p *Processor) run() {
	defer func() {
		p.done <- struct{}{}
	}()

	for {
		select {
		case msg, ok := <-p.inputChan:
			if !ok { // channel has been closed
				return
			}
			p.processMessage(msg)
			p.mu.Lock() // block here if we're trying to flush synchronously
			//nolint:staticcheck
			p.mu.Unlock()
		case order := <-p.ReconfigChan:
			p.mu.Lock()
			if err := p.sds.Reconfigure(order); err != nil {
				log.Errorf("Error while reconfiguring the SDS scanner: %v", err)
				order.ResponseChan <- err
			} else {
			    p.flareCtl.SetStandardSDSRules([]string{})
			    p.flareCtl.SetEnabledSDSRules([]string{})
			    if p.sds.IsReady() {
					p.flareCtl.SetStandardSDSRules(p.sds.GetStandardRulesNames())
					p.flareCtl.SetEnabledSDSRules(p.sds.GetEnabledRulesNames())
				    p.flareCtl.SetSDSLastReconfiguration(time.Now())
				}
				order.ResponseChan <- nil
			}
			p.mu.Unlock()
		}
	}
}

func (p *Processor) processMessage(msg *message.Message) {
	metrics.LogsDecoded.Add(1)
	metrics.TlmLogsDecoded.Inc()

	if toSend := p.applyRedactingRules(msg); toSend {
		metrics.LogsProcessed.Add(1)
		metrics.TlmLogsProcessed.Inc()

		// render the message
		rendered, err := msg.Render()
		if err != nil {
			log.Error("can't render the msg", err)
			return
		}
		msg.SetRendered(rendered)

		// report this message to diagnostic receivers (e.g. `stream-logs` command)
		p.diagnosticMessageReceiver.HandleMessage(msg, rendered, "")

		// encode the message to its final format, it is done in-place
		if err := p.encoder.Encode(msg, p.GetHostname(msg)); err != nil {
			log.Error("unable to encode msg ", err)
			return
		}

		p.outputChan <- msg
	}
}

// applyRedactingRules returns given a message if we should process it or not,
// it applies the change directly on the Message content.
func (p *Processor) applyRedactingRules(msg *message.Message) bool {
	var content []byte = msg.GetContent()

	// Use the internal scrubbing implementation of the Agent
	// ---------------------------

	rules := append(p.processingRules, msg.Origin.LogSource.Config.ProcessingRules...)
	for _, rule := range rules {
		switch rule.Type {
		case config.ExcludeAtMatch:
			// if this message matches, we ignore it
			if rule.Regex.Match(content) {
				return false
			}
		case config.IncludeAtMatch:
			// if this message doesn't match, we ignore it
			if !rule.Regex.Match(content) {
				return false
			}
		case config.MaskSequences:
			content = rule.Regex.ReplaceAll(content, rule.Placeholder)
		}
	}

	// Use the SDS implementation
	// --------------------------

	// Global SDS scanner, applied on all log sources
	if p.sds.IsReady() {
		matched, processed, err := p.sds.Scan(content, msg)
		if err != nil {
			log.Error("while using SDS to scan the log:", err)
		} else if matched {
			content = processed
		}
	}

	msg.SetContent(content)
	return true // we want to send this message
}

// GetHostname returns the hostname to applied the given log message
func (p *Processor) GetHostname(msg *message.Message) string {
	if msg.Hostname != "" {
		return msg.Hostname
	}

	if msg.Lambda != nil {
		return msg.Lambda.ARN
	}

	if p.hostname == nil {
		return "unknown"
	}
	hname, err := p.hostname.Get(context.TODO())
	if err != nil {
		// this scenario is not likely to happen since
		// the agent cannot start without a hostname
		hname = "unknown"
	}
	return hname
}
