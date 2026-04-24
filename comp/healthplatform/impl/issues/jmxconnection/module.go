// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package jmxconnection provides an issue module for JMX connection failures.
// This module only provides remediation (no built-in check) as JMX connection
// failures are reported by the JMXFetch check runner.
package jmxconnection

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for JMX connection failure issues
	IssueID = "jmx-connection-failure"
)

// jmxConnectionModule implements issues.Module
type jmxConnectionModule struct {
	template *JMXConnectionIssue
}

// NewModule creates a new JMX connection failure issue module
func NewModule(config.Component) issues.Module {
	return &jmxConnectionModule{
		template: NewJMXConnectionIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *jmxConnectionModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *jmxConnectionModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - JMX connection failures are reported by the JMXFetch check runner
func (m *jmxConnectionModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
