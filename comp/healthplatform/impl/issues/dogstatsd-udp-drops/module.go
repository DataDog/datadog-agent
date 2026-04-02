// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dogstatsdudpdrops provides a health platform issue module that detects
// when the DogStatsD UDP receive buffer is left at the OS default (0), which can
// cause silent UDP packet drops under high metric throughput.
package dogstatsdudpdrops

import "github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for the DogStatsD UDP buffer undersized issue
	IssueID = "dogstatsd-udp-buffer-undersized"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "dogstatsd-udp-buffer-config"

	// CheckName is the human-readable name for the health check
	CheckName = "DogStatsD UDP Receive Buffer Configuration"
)

// module implements issues.Module
type module struct {
	template *Issue
}

// NewModule creates a new DogStatsD UDP drops issue module
func NewModule() issues.Module {
	return &module{
		template: NewIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *module) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *module) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil because detection happens inline in the DogStatsD
// UDP listener — ReportIssue is called directly when a read error occurs.
func (m *module) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
