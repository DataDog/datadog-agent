// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clustering provides clustering functionality for grouping similar TokenLists
// and identifying wildcard positions for pattern extraction.
package clustering

import "time"

// PatternTemplateStatus indicates whether the pattern template needs to be sent
type PatternTemplateStatus int

const (
	// TemplateNotNeeded indicates template is already synced, no action needed
	TemplateNotNeeded PatternTemplateStatus = iota
	// TemplateIsNew indicates template has never been sent, needs PatternDefine
	TemplateIsNew
	// TemplateChanged indicates template changed since last send, needs PatternDelete + PatternDefine
	TemplateChanged
)

// NeedsResend determines if a pattern template needs to be sent and its status.
// Returns (needsSend, templateStatus):
// - (false, TemplateNotNeeded) if template was already sent and hasn't changed
// - (true, TemplateIsNew) if template has never been sent
// - (true, TemplateChanged) if template changed since last send
func (p *Pattern) NeedsResend() (bool, PatternTemplateStatus) {
	if p == nil {
		return false, TemplateNotNeeded
	}

	// Never sent? Need to send as new template
	if p.LastSentAt.IsZero() {
		return true, TemplateIsNew
	}

	// Check if template changed since last send
	currentTemplate := p.GetPatternString()
	if p.SentTemplate != currentTemplate {
		return true, TemplateChanged
	}

	// Already sent and unchanged
	return false, TemplateNotNeeded
}

// MarkAsSent records that this pattern was successfully sent.
// It updates both the LastSentAt timestamp and stores the sent template.
func (p *Pattern) MarkAsSent() {
	if p == nil {
		return
	}
	p.LastSentAt = time.Now()
	p.SentTemplate = p.GetPatternString()
}
