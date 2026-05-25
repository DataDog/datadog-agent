// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ntpdrift provides a complete issue module for NTP clock drift detection.
// It includes both detection (built-in periodic health check) and remediation
// (issue template with fix steps).
package ntpdrift

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for NTP clock drift issues
	IssueID = "ntp-clock-drift"

	// checkInterval is how often the NTP drift check runs
	checkInterval = 15 * time.Minute
)

// ntpDriftModule implements issues.Module
type ntpDriftModule struct {
	template *NTPDriftIssue
}

// NewModule creates a new NTP drift issue module
func NewModule(_ config.Component) issues.Module {
	return &ntpDriftModule{
		template: NewNTPDriftIssue(),
	}
}

// IssueType returns the unique identifier for this issue type
func (m *ntpDriftModule) IssueType() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *ntpDriftModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInPeriodicHealthCheck returns the periodic health check for NTP drift.
func (m *ntpDriftModule) BuiltInPeriodicHealthCheck() *issues.BuiltInPeriodicHealthCheck {
	return &issues.BuiltInPeriodicHealthCheck{
		Source:   "ntp-drift",
		Fn:       Check,
		Interval: checkInterval,
	}
}

// BuiltInStartupHealthCheck returns nil — NTP drift uses a periodic check only.
func (m *ntpDriftModule) BuiltInStartupHealthCheck() *issues.BuiltInStartupHealthCheck {
	return nil
}
