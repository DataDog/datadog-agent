// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
	"context"
	"regexp"
	"slices"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	statusutils "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
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
	tagFilters                *tagfilter.Filters
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
	tagFilters *tagfilter.Filters, encoder Encoder, diagnosticMessageReceiver diagnostic.MessageReceiver, hostname hostnameinterface.Component,
	pipelineMonitor metrics.PipelineMonitor, instanceID string) *Processor {

	p := &Processor{
		config:                    config,
		inputChan:                 inputChan,
		outputChan:                outputChan, // strategy input
		processingRules:           processingRules,
		tagFilters:                tagFilters,
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
func (p *Processor) onLogsFailoverSettingChanged(setting string, _ pkgconfigmodel.Source, _, _ any, _ uint64, _ pkgconfigmodel.Source) {
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

		p.resolveTagFilter(msg)

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

// ResolveSourceTagFilter compiles src's tag filters against the global set, records
// any problems on src, registers its status block, and caches the result on src.
//
// Safe to call more than once for the same source, including concurrently: once
// resolved, later calls return immediately, and racing calls converge on the same
// value since global and src's config are immutable.
func ResolveSourceTagFilter(global *tagfilter.Filters, src *sources.LogSource) {
	// LogSources.SubscribeAll replays every source it holds, including ones AddSource
	// appended before rejecting for a nil Config.
	if src == nil || src.Config == nil {
		return
	}
	if _, ok := src.TagFilter(); ok {
		return
	}

	sourceFilters, report := src.Config.TagFilters.Compile()
	// A malformed pattern degrades to filtering less, never blocks the source
	// from tailing; record it so it's visible on the source's status block.
	for _, rejected := range report.Rejected {
		src.Messages.AddMessage("tag_filters:rejected:"+rejected.Pattern, rejected.Reason)
	}
	for _, warning := range report.Warnings {
		src.Messages.AddMessage("tag_filters:warning:"+warning, warning)
	}

	globalPatterns := global.Patterns()
	sourcePatterns := sourceFilters.Patterns()
	if !globalPatterns.IsEmpty() || !sourcePatterns.IsEmpty() {
		src.RegisterInfo(statusutils.NewTagFilterInfo(
			globalPatterns.Include, globalPatterns.Exclude,
			sourcePatterns.Include, sourcePatterns.Exclude))
	}

	// Typed-nil trap: NewScoped can return a nil *Scoped. Only assign resolved when
	// it doesn't, so a fully-unfiltered source stamps a genuinely nil
	// sources.TagFilter rather than a non-nil interface wrapping a nil pointer.
	var resolved sources.TagFilter
	if scoped := tagfilter.NewScoped(global, sourceFilters); scoped != nil {
		resolved = scoped
	}
	src.SetTagFilter(resolved)
}

// resolveTagFilter stamps msg with the tag filter for its source, resolving it via
// ResolveSourceTagFilter if the logs agent's eager subscriber hasn't already.
func (p *Processor) resolveTagFilter(msg *message.Message) {
	if msg.Origin == nil || msg.Origin.LogSource == nil {
		return
	}
	src := msg.Origin.LogSource

	f, ok := src.TagFilter()
	if !ok {
		ResolveSourceTagFilter(p.tagFilters, src)
		f, _ = src.TagFilter()
	}
	msg.SetTagFilter(f)
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
