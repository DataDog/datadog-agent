// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ntpdrift provides a complete issue module for NTP clock drift detection.
// It includes both detection (built-in health check) and remediation (issue template with fix steps).
package ntpdrift

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for NTP clock drift issues
	IssueID = "ntp-clock-drift"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "ntp-drift"

	// CheckName is the human-readable name for the health check
	CheckName = "NTP Clock Drift"
)

// ntpDriftModule implements issues.Module
type ntpDriftModule struct {
	template *NTPDriftIssue
}

// NewModule creates a new NTP drift issue module
func NewModule(config.Component) issues.Module {
	return &ntpDriftModule{
		template: NewNTPDriftIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *ntpDriftModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *ntpDriftModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration.
// Interval is 0 to use the default (15 minutes).
func (m *ntpDriftModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:      CheckID,
		Name:    CheckName,
		CheckFn: Check,
	}
}
