// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package awshostnamemismatch provides an issue module for AWS EC2 hostname mismatches.
// It includes a startup health check that fires when the agent's configured hostname
// does not match or contain the EC2 instance ID, which can cause metric attribution issues.
package awshostnamemismatch

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for AWS hostname mismatch issues
	IssueID = "aws-hostname-mismatch"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "aws-hostname"

	// CheckName is the human-readable name for the health check
	CheckName = "AWS EC2 Hostname Check"
)

// awsHostnameMismatchModule implements issues.Module
type awsHostnameMismatchModule struct {
	cfg      config.Component
	template *AWSHostnameMismatchIssue
}

// NewModule creates a new AWS hostname mismatch issue module
func NewModule(cfg config.Component) issues.Module {
	return &awsHostnameMismatchModule{
		cfg:      cfg,
		template: NewAWSHostnameMismatchIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *awsHostnameMismatchModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *awsHostnameMismatchModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration.
// Once is true so it runs once at startup.
func (m *awsHostnameMismatchModule) BuiltInCheck() *issues.BuiltInCheck {
	cfg := m.cfg
	return &issues.BuiltInCheck{
		ID:   CheckID,
		Name: CheckName,
		CheckFn: func() (*healthplatform.IssueReport, error) {
			return Check(cfg)
		},
		Once: true,
	}
}
