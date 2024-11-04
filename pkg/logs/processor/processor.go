// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
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
	mu                        sync.Mutex
	hostname                  hostnameinterface.Component

	sds sdsProcessor

	// Telemetry
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
}

type sdsProcessor struct {
	// buffer stores the messages for the buffering mechanism in case we didn't
	// receive any SDS configuration & wait_for_configuration == "buffer".
	buffer        []*message.Message
	bufferedBytes int
	maxBufferSize int

	/// buffering indicates if we're buffering while waiting for an SDS configuration
	buffering bool

	scanner *sds.Scanner // configured through RC
}

// New returns an initialized Processor.
func New(cfg pkgconfigmodel.Reader, inputChan, outputChan chan *message.Message, processingRules []*config.ProcessingRule,
	encoder Encoder, diagnosticMessageReceiver diagnostic.MessageReceiver, hostname hostnameinterface.Component,
	pipelineMonitor metrics.PipelineMonitor) *Processor {

	waitForSDSConfig := sds.ShouldBufferUntilSDSConfiguration(cfg)
	maxBufferSize := sds.WaitForConfigurationBufferMaxSize(cfg)

	return &Processor{
		inputChan:                 inputChan,
		outputChan:                outputChan, // strategy input
		ReconfigChan:              make(chan sds.ReconfigureOrder),
		processingRules:           processingRules,
		encoder:                   encoder,
		done:                      make(chan struct{}),
		diagnosticMessageReceiver: diagnosticMessageReceiver,
		hostname:                  hostname,
		pipelineMonitor:           pipelineMonitor,
		utilization:               pipelineMonitor.MakeUtilizationMonitor("processor"),

		sds: sdsProcessor{
			// will immediately starts buffering if it has been configured as so
			buffering:     waitForSDSConfig,
			maxBufferSize: maxBufferSize,
			scanner:       sds.CreateScanner(pipelineMonitor.ID()),
		},
	}
}

// Start starts the Processor.
func (p *Processor) Start() {
	go p.run()
}

// Stop stops the Processor,
// this call blocks until inputChan is flushed
func (p *Processor) Stop() {
	close(p.inputChan)
	<-p.done
	// once the processor mainloop is not running, it's safe
	// to delete the sds scanner instance.
	if p.sds.scanner != nil {
		p.sds.scanner.Delete()
		p.sds.scanner = nil
	}
}

// Flush processes synchronously the messages that this processor has to process.
// Mainly (only?) used by the Serverless Agent.
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
			p.pipelineMonitor.ReportComponentIngress(msg, "processor")
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
		// Processing, usual main loop
		// ---------------------------

		case msg, ok := <-p.inputChan:
			if !ok { // channel has been closed
				return
			}

			// if we have to wait for an SDS configuration to start processing & forwarding
			// the logs, that's here that we buffer the message
			if p.sds.buffering {
				// buffer until we receive a configuration
				p.sds.bufferMsg(msg)
			} else {
				// process the message
				p.processMessage(msg)
			}

			p.mu.Lock() // block here if we're trying to flush synchronously
			//nolint:staticcheck
			p.mu.Unlock()

		// SDS reconfiguration
		// -------------------

		case order := <-p.ReconfigChan:
			p.mu.Lock()
			p.applySDSReconfiguration(order)
			p.mu.Unlock()
		}
	}
}

func (p *Processor) applySDSReconfiguration(order sds.ReconfigureOrder) {
	isActive, err := p.sds.scanner.Reconfigure(order)
	response := sds.ReconfigureResponse{
		IsActive: isActive,
		Err:      err,
	}

	if err != nil {
		log.Errorf("Error while reconfiguring the SDS scanner: %v", err)
	} else {
		// no error while reconfiguring the SDS scanner and since it looks active now,
		// we should drain the buffered messages if any and stop the
		// buffering mechanism.
		if p.sds.buffering && isActive {
			log.Debug("Processor ready with an SDS configuration.")
			p.sds.buffering = false

			// drain the buffer of messages if anything's in there
			if len(p.sds.buffer) > 0 {
				log.Info("SDS: sending", len(p.sds.buffer), "buffered messages")
				for _, msg := range p.sds.buffer {
					p.processMessage(msg)
				}
			}

			p.sds.resetBuffer()
		}
		// no else case, the buffering is only a startup mechanism, after having
		// enabled the SDS scanners, if they become inactive it is because the
		// configuration has been sent like that.
	}

	order.ResponseChan <- response
}

func (s *sdsProcessor) bufferMsg(msg *message.Message) {
	s.buffer = append(s.buffer, msg)
	s.bufferedBytes += len(msg.GetContent())

	for len(s.buffer) > 0 {
		if s.bufferedBytes > s.maxBufferSize {
			s.bufferedBytes -= len(s.buffer[0].GetContent())
			s.buffer = s.buffer[1:]
			metrics.TlmLogsDiscardedFromSDSBuffer.Inc()
		} else {
			break
		}
	}
}

func (s *sdsProcessor) resetBuffer() {
	s.buffer = nil
	s.bufferedBytes = 0
}

func (p *Processor) processMessage(msg *message.Message) {
	p.utilization.Start()
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

		p.utilization.Stop()
		p.outputChan <- msg
		p.pipelineMonitor.ReportComponentIngress(msg, "strategy")
	} else {
		p.utilization.Stop()
	}
	p.pipelineMonitor.ReportComponentEgress(msg, "processor")

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
	if p.sds.scanner.IsReady() {
		mutated, evtProcessed, err := p.sds.scanner.Scan(content, msg)
		if err != nil {
			log.Error("while using SDS to scan the log:", err)
		} else if mutated {
			content = evtProcessed
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
