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

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueName is the identifier for NTP clock drift issues,
	// used as the template registry key and the proto IssueName field.
	IssueName = "NTP Clock Drift"

	// IssueID is the unique instance id used when reporting this issue
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

func (m *ntpDriftModule) IssueName() string {
	return IssueName
}

func (m *ntpDriftModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns the periodic health check for NTP drift.
func (m *ntpDriftModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: "ntp-drift",
			Fn:     Check,
		},
		Interval: checkInterval,
	}
}

// BuiltInStartupHealthCheck returns nil — NTP drift uses a periodic check only.
func (m *ntpDriftModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
