// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package jmxmetriccap provides an issue module for JMX metric collection limit exceeded.
// This module only provides remediation (no built-in check) as JMX metric cap events
// are reported by external integrations (JMXFetch).
package jmxmetriccap

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for JMX metric cap issues
	IssueID = "jmx-metric-limit-reached"
)

// jmxMetricCapModule implements issues.Module
type jmxMetricCapModule struct {
	template *JMXMetricCapIssue
}

// NewModule creates a new JMX metric cap issue module
func NewModule(config.Component) issues.Module {
	return &jmxMetricCapModule{
		template: NewJMXMetricCapIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *jmxMetricCapModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *jmxMetricCapModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - JMX metric cap events are reported by external integrations
func (m *jmxMetricCapModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
