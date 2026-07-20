// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"sync"

	"go.opentelemetry.io/collector/pdata/plog"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
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
	rules := append(p.processingRules, extraRules...)
	for _, rule := range rules {
		switch rule.Type {

		case config.ExcludeAtMatchOTTL:
			if p.processOTTLmsg(msg, rule) {
				return false
			}

		case config.IncludeAtMatchOTTL:
			if !p.processOTTLmsg(msg, rule) {
				return false
			}

		case config.ExcludeAtMatch:
			// if this message matches, we ignore it
			if rule.Regex.Match(content) {
				msg.RecordProcessingRule(rule.Type, rule.Name)
				return false
			}
		case config.IncludeAtMatch:
			// if this message doesn't match, we ignore it
			if !rule.Regex.Match(content) {
				return false
			}
			msg.RecordProcessingRule(rule.Type, rule.Name)
		case config.MaskSequences:
			if isMatchingLiteralPrefix(rule.Regex, content) {
				originalContent := content
				content = rule.Regex.ReplaceAll(content, rule.Placeholder)
				if !bytes.Equal(originalContent, content) {
					msg.RecordProcessingRule(rule.Type, rule.Name)
				}
			}
		case config.ExcludeTruncated:
			if msg.IsTruncated {
				msg.RecordProcessingRule(rule.Type, rule.Name)
				return false
			}
		case config.ExcludeAtJQMatch:
			if matched, err := rule.JQFilter(content); err == nil && matched {
				msg.RecordProcessingRule(rule.Type, rule.Name)
				return false
			}
		case config.IncludeAtJQMatch:
			matched, err := rule.JQFilter(content)
			if err != nil {
				// Non-JSON or runtime error: pass through unchanged.
				break
			}
			if !matched {
				return false
			}
			msg.RecordProcessingRule(rule.Type, rule.Name)
		case config.ExcludeAtVRLMatch:
			// if this message matches, we ignore it. Fail open (keep the message)
			// on a runtime error, since a filter false-negative doesn't leak data.
			matched, err := rule.VRLFilter(content)
			if err != nil {
				metrics.TlmVRLEvalErrors.Inc(rule.Type, rule.Name)
				break
			}
			if matched {
				msg.RecordProcessingRule(rule.Type, rule.Name)
				return false
			}
		case config.IncludeAtVRLMatch:
			// if this message doesn't match, we ignore it. Fail open (keep the
			// message) on a runtime error.
			matched, err := rule.VRLFilter(content)
			if err != nil {
				metrics.TlmVRLEvalErrors.Inc(rule.Type, rule.Name)
				break
			}
			if !matched {
				return false
			}
			msg.RecordProcessingRule(rule.Type, rule.Name)
		case config.MaskVRLTransform:
			newContent, err := rule.VRLTransform(content)
			if err != nil {
				// Fail closed: drop the message rather than risk leaking
				// unredacted PII when the VRL transform can't run.
				metrics.TlmVRLEvalErrors.Inc(rule.Type, rule.Name)
				msg.RecordProcessingRule(rule.Type, rule.Name)
				return false
			}
			if !bytes.Equal(content, newContent) {
				msg.RecordProcessingRule(rule.Type, rule.Name)
			}
			content = newContent
		case config.RemapSource:
			for _, match := range rule.Matching {
				if val, ok := msg.GetStructuredAttribute(match.Attribute); ok && val == match.Value {
					if msg.Origin != nil {
						msg.Origin.SetMappedSource(match.NewSource)
					}
					msg.RecordProcessingRule(rule.Type, rule.Name)
					break
				}
			}
		}
	}

	msg.SetContent(content)
	return true // we want to send this message
}

// isMatchingLiteralPrefix uses a potential literal prefix from the given regex
// to indicate if the contant even has a chance of matching the regex
func isMatchingLiteralPrefix(r *regexp.Regexp, content []byte) bool {
	prefix, _ := r.LiteralPrefix()
	if prefix == "" {
		return true
	}

	return bytes.Contains(content, []byte(prefix))
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

// processOTTLmsg
func (p *Processor) processOTTLmsg(msg *message.Message, rule *config.ProcessingRule) bool {
	// If the OTTL condition is not compiled, we log an error and skip OTTL processing for this rule
	if rule.OTTLCondition == nil {
		log.Errorf("OTTL condition is not compiled for rule `%s`, skipping OTTL processing", rule.Name)
		return false
	}

	var m map[string]any

	err := json.Unmarshal(msg.GetContent(), &m)
	if err != nil {
		log.Errorf("error unmarshalling message content for OTTL processing rule `%s`: %v", rule.Name, err)
		return false
	}

	pdata := plog.NewLogRecord()
	for k, v := range m {
		switch val := v.(type) {
		case string:
			pdata.Attributes().PutStr(k, val)
		case bool:
			pdata.Attributes().PutBool(k, val)
		case float64:
			pdata.Attributes().PutDouble(k, val)
		default:
			log.Warnf("unsupported attribute type for key `%s` in OTTL processing rule `%s`, skipping this attribute", k, rule.Name)
		}
	}

	rl := plog.NewResourceLogs()
	sl := rl.ScopeLogs().AppendEmpty()

	transformContext := ottllog.NewTransformContextPtr(rl, sl, pdata)
	defer transformContext.Close()

	match, err := rule.OTTLCondition.Eval(context.Background(), transformContext)
	if err != nil {
		log.Errorf("error evaluating OTTL condition for processing rule `%s`: %v", rule.Name, err)
		return false
	}

	if match {
		msg.RecordProcessingRule(rule.Type, rule.Name)
	}

	return match

}
