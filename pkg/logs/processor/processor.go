// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"context"
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// UnstructuredProcessingMetricName collects how many rules are used on unstructured
	// content for tailers capable of processing both unstructured and structured content.
	UnstructuredProcessingMetricName = "datadog.logs_agent.tailer.unstructured_processing"

	// MRF logs settings
	configMRFFailoverLogs     = "multi_region_failover.failover_logs"
	configMRFServiceAllowlist = "multi_region_failover.logs_service_allowlist"
)

type failoverConfig struct {
	isFailoverActive         bool
	failoverServiceAllowlist map[string]struct{}
}

// A Processor updates messages from an inputChan and pushes
// in an outputChan.
type Processor struct {
	inputChan                 chan *message.Message
	outputChan                chan *message.Message // strategy input
	processingRules           []*config.ProcessingRule
	encoder                   Encoder
	done                      chan struct{}
	diagnosticMessageReceiver diagnostic.MessageReceiver
	mu                        sync.Mutex
	hostname                  hostnameinterface.Component
	config                    pkgconfigmodel.Reader
	configChan                chan failoverConfig
	failoverConfig            failoverConfig

	// Telemetry
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
	instanceID      string
}

// New returns an initialized Processor with config support for failover notifications.
func New(config pkgconfigmodel.Reader, inputChan, outputChan chan *message.Message, processingRules []*config.ProcessingRule,
	encoder Encoder, diagnosticMessageReceiver diagnostic.MessageReceiver, hostname hostnameinterface.Component,
	pipelineMonitor metrics.PipelineMonitor, instanceID string) *Processor {

	p := &Processor{
		config:                    config,
		inputChan:                 inputChan,
		outputChan:                outputChan, // strategy input
		processingRules:           processingRules,
		encoder:                   encoder,
		configChan:                make(chan failoverConfig, 1),
		done:                      make(chan struct{}),
		diagnosticMessageReceiver: diagnosticMessageReceiver,
		hostname:                  hostname,
		pipelineMonitor:           pipelineMonitor,
		utilization:               pipelineMonitor.MakeUtilizationMonitor(metrics.ProcessorTlmName, instanceID),
		instanceID:                instanceID,
	}

	// Initialize cached failover config
	p.updateFailoverConfig()

	// Register for config change notifications
	if config != nil {
		config.OnUpdate(p.onLogsFailoverSettingChanged)
	}

	return p
}

// onLogsFailoverSettingChanged is called when any config value changes
func (p *Processor) onLogsFailoverSettingChanged(setting string, _ pkgconfigmodel.Source, _, _ any, _ uint64) {
	// Only update if the changed setting affects failover configuration
	var MRFConfigFields = []string{configMRFFailoverLogs, configMRFServiceAllowlist}
	if slices.Contains(MRFConfigFields, setting) {
		p.updateFailoverConfig()
	}
}

// updateFailoverConfig sends the updated config to the processor to update
func (p *Processor) updateFailoverConfig() {
	if p.config == nil {
		return
	}

	conf := failoverConfig{
		isFailoverActive: p.config.GetBool(configMRFFailoverLogs),
	}

	var serviceAllowlist map[string]struct{}
	if conf.isFailoverActive && p.config.IsConfigured(configMRFServiceAllowlist) {
		rawList := p.config.GetStringSlice(configMRFServiceAllowlist)
		serviceAllowlist = make(map[string]struct{}, len(rawList))
		for _, allowed := range rawList {
			serviceAllowlist[allowed] = struct{}{}
		}

		conf.failoverServiceAllowlist = serviceAllowlist
	}

	p.configChan <- conf
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
			if !ok {
				return
			}
			p.processMessage(msg)
			p.mu.Lock() // block here if we're trying to flush synchronously
			//nolint:staticcheck
			p.mu.Unlock()
		case conf := <-p.configChan:
			p.failoverConfig = conf
		}
	}
}

func (p *Processor) processMessage(msg *message.Message) {
	p.utilization.Start()
	defer p.utilization.Stop()
	defer p.pipelineMonitor.ReportComponentEgress(msg, metrics.ProcessorTlmName, p.instanceID)
	metrics.LogsDecoded.Add(1)
	metrics.TlmLogsDecoded.Inc()
	// Record truncation metrics if the message is truncated
	if msg.ParsingExtra.IsTruncated {
		if msg.Origin != nil {
			metrics.TlmTruncatedCount.Inc(msg.Origin.Service(), msg.Origin.Source())
		} else {
			metrics.TlmTruncatedCount.Inc("", "")
		}
	}

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

		if p.failoverConfig.isFailoverActive {
			p.filterMRFMessages(msg)
		}

		// encode the message to its final format, it is done in-place
		if err := p.encoder.Encode(msg, p.GetHostname(msg)); err != nil {
			log.Error("unable to encode msg ", err)
			return
		}

		p.utilization.Stop() // Explicitly call stop here to avoid counting writing on the output channel as processing time
		p.outputChan <- msg
		p.pipelineMonitor.ReportComponentIngress(msg, metrics.StrategyTlmName, p.instanceID)
	}
}

// filterMRFMessages applies an MRF tag to messages that should be sent to MRF
// destinations
func (p *Processor) filterMRFMessages(msg *message.Message) {
	serviceAllowlist := p.failoverConfig.failoverServiceAllowlist

	// Tag the message for failover if:
	// 1. No allowlists are configured (i.e., failover everything).
	// 2. The message service is in the service allowlist.
	if len(serviceAllowlist) == 0 {
		msg.IsMRFAllow = true
		return
	}

	_, serviceMatch := serviceAllowlist[msg.Origin.Service()]
	if serviceMatch {
		msg.IsMRFAllow = true
		return
	}
}

// applyRedactingRules returns given a message if we should process it or not,
// it applies the change directly on the Message content.
func (p *Processor) applyRedactingRules(msg *message.Message) bool {
	var content = msg.GetContent()

	// Use the internal scrubbing implementation of the Agent
	// ---------------------------

	var extraRules []*config.ProcessingRule
	if msg.Origin != nil && msg.Origin.LogSource != nil {
		extraRules = msg.Origin.LogSource.Config.ProcessingRules
	}
	for _, rule := range p.processingRules {
		content = applyRule(rule, msg, content)
		if content == nil {
			return false
		}
	}

	for _, rule := range extraRules {
		content = applyRule(rule, msg, content)
		if content == nil {
			return false
		}
	}
	msg.SetContent(content)
	return true // we want to send this message
}

// applyRule applies a single processing rule to content and returns the
// (possibly modified) content, or nil if the message should be dropped.
func applyRule(rule *config.ProcessingRule, msg *message.Message, content []byte) []byte {
	switch rule.Type {
	case config.ExcludeAtMatch:
		if re2MatchContent(rule, content) {
			msg.RecordProcessingRule(rule.GetDesignation())
			return nil
		}
	case config.IncludeAtMatch:
		if !re2MatchContent(rule, content) {
			return nil
		}
		msg.RecordProcessingRule(rule.GetDesignation())
	case config.MaskSequences:
		if result, matched := re2MaskReplace(rule, content); matched {
			content = result
			msg.RecordProcessingRule(rule.GetDesignation())
		}
	case config.ExcludeTruncated:
		if msg.IsTruncated {
			msg.RecordProcessingRule(rule.GetDesignation())
			return nil
		}
	}
	return content
}

// GetHostname returns the hostname to applied the given log message
func (p *Processor) GetHostname(msg *message.Message) string {
	if msg.Hostname != "" {
		return msg.Hostname
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
