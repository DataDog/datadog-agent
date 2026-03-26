// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package awsimds provides a complete issue module for AWS IMDSv2 hop limit problems.
// It detects when the agent running in a container cannot reach the AWS instance
// metadata service due to the default IMDSv2 hop limit of 1, which prevents
// container traffic from traversing the extra network hop to the metadata endpoint.
package awsimds

import (
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for AWS IMDS hop limit issues
	IssueID = "aws-imds-hop-limit"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "aws-imds-connectivity"

	// CheckName is the human-readable name for the health check
	CheckName = "AWS IMDS Connectivity"
)

// awsIMDSModule implements issues.Module
type awsIMDSModule struct {
	template *AWSIMDSIssue
}

// NewModule creates a new AWS IMDS hop limit issue module
func NewModule() issues.Module {
	return &awsIMDSModule{
		template: NewAWSIMDSIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *awsIMDSModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *awsIMDSModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration
// Interval is 0 to use the default (15 minutes)
func (m *awsIMDSModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:      CheckID,
		Name:    CheckName,
		CheckFn: Check,
	}
}
