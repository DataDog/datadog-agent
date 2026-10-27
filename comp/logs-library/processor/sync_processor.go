// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package processor

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// SyncProcessor applies processing rules to messages, renders and encodes them in the calling
// goroutine, the same way Processor does in its own goroutine. It serves callers that need the
// outcome of every message. It does not tag messages for Multi-Region Failover, and it is safe for
// concurrent use.
type SyncProcessor struct {
	processingRules []*config.ProcessingRule
	encoder         Encoder
	hostname        hostnameinterface.Component
}

// NewSyncProcessor returns a SyncProcessor.
func NewSyncProcessor(processingRules []*config.ProcessingRule, encoder Encoder, hostname hostnameinterface.Component) *SyncProcessor {
	return &SyncProcessor{
		processingRules: processingRules,
		encoder:         encoder,
		hostname:        hostname,
	}
}

// Process prepares msg for sending, in place. It returns false when a processing rule filtered the
// message out.
func (p *SyncProcessor) Process(msg *message.Message) (bool, error) {
	metrics.LogsDecoded.Add(1)
	metrics.TlmLogsDecoded.Inc()
	if !applyProcessingRules(msg, p.processingRules) {
		return false, nil
	}
	metrics.LogsProcessed.Add(1)
	metrics.TlmLogsProcessed.Inc()

	rendered, err := msg.Render()
	if err != nil {
		return false, fmt.Errorf("can't render the message: %w", err)
	}
	msg.SetRendered(rendered)
	if err := p.encoder.Encode(msg, hostnameFor(msg, p.hostname)); err != nil {
		return false, fmt.Errorf("unable to encode the message: %w", err)
	}
	return true, nil
}
