// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package statusimpl implements the health platform status provider.
package statusimpl

import (
	"embed"
	"fmt"
	"io"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	corestatus "github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	egressdef "github.com/DataDog/datadog-agent/comp/healthplatform/egress/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// team: fleet-remediation

//go:embed status_templates
var templatesFS embed.FS

// Requires defines the dependencies for the health platform status component.
type Requires struct {
	Config config.Component
	Store  storedef.Component
	Egress egressdef.Component
}

// Provides defines the output of the health platform status component.
type Provides struct {
	compdef.Out

	StatusProvider corestatus.InformationProvider
}

type statusProvider struct {
	config config.Component
	store  storedef.Component
	egress egressdef.Component
}

// NewComponent creates a new health platform status component.
func NewComponent(reqs Requires) Provides {
	return Provides{
		StatusProvider: corestatus.NewInformationProvider(statusProvider{
			config: reqs.Config,
			store:  reqs.Store,
			egress: reqs.Egress,
		}),
	}
}

// Name returns the name
func (s statusProvider) Name() string {
	return "Health Platform"
}

// Section returns the section
func (s statusProvider) Section() string {
	return "Health Platform"
}

// JSON populates the status map
func (s statusProvider) JSON(_ bool, stats map[string]interface{}) error {
	s.populateStatus(stats)
	return nil
}

// Text renders the text output
func (s statusProvider) Text(_ bool, buffer io.Writer) error {
	return corestatus.RenderText(templatesFS, "healthplatform.tmpl", buffer, s.getStatusInfo())
}

// HTML renders the html output
func (s statusProvider) HTML(_ bool, buffer io.Writer) error {
	return corestatus.RenderHTML(templatesFS, "healthplatformHTML.tmpl", buffer, s.getStatusInfo())
}

func (s statusProvider) getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})
	s.populateStatus(stats)
	return stats
}

// populateStatus reports active-issue counts by severity and the health of
// the egress send pipeline. It intentionally does not list individual
// issues: `agent diagnose` already does that, and duplicating it here would
// just be two places to keep in sync.
func (s statusProvider) populateStatus(stats map[string]interface{}) {
	hp := make(map[string]interface{})
	stats["healthPlatform"] = hp

	if !s.config.GetBool("health_platform.enabled") {
		hp["enabled"] = false
		return
	}
	hp["enabled"] = true

	count, issues := s.store.GetAllIssues()
	var high, medium, low, unknown int
	for _, issue := range issues {
		switch issue.GetSeverity() {
		case healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH:
			high++
		case healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_MEDIUM:
			medium++
		case healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_LOW:
			low++
		default:
			unknown++
		}
	}
	hp["activeIssues"] = count
	hp["highSeverityIssues"] = high
	hp["mediumSeverityIssues"] = medium
	hp["lowSeverityIssues"] = low
	hp["unknownSeverityIssues"] = unknown

	egressStatus := s.egress.Status()
	hp["egressHealthy"] = egressStatus.Healthy
	hp["issuesSentTotal"] = egressStatus.IssuesSentTotal
	hp["bytesSentTotal"] = egressStatus.BytesSentTotal
	hp["sendErrorsTotal"] = egressStatus.SendErrorsTotal
	if !egressStatus.LastSuccessAt.IsZero() {
		hp["lastSuccessAt"] = egressStatus.LastSuccessAt.Format(time.RFC1123)
	}
	if egressStatus.LastError != nil {
		hp["lastError"] = fmt.Sprintf("%v", egressStatus.LastError)
	}
}
