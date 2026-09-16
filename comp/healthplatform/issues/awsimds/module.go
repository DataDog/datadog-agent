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
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueName is the identifier for AWS IMDS hop limit issues,
	// used as the template registry key and the proto IssueName field.
	IssueName = "AWS IMDS Hop Limit"

	// IssueType is the snake_case type key for AWS IMDS hop limit issues:
	// IssueName lowercased with spaces replaced by underscores.
	IssueType = "aws_imds_hop_limit"

	// IssueID is the unique instance id used when reporting this issue
	IssueID = "aws-imds-hop-limit"
)

// awsIMDSModule implements issues.Module
type awsIMDSModule struct {
	template *AWSIMDSIssue
}

// NewModule creates a new AWS IMDS hop limit issue module
func NewModule(issues.ModuleDeps) issues.Module {
	return &awsIMDSModule{
		template: NewAWSIMDSIssue(),
	}
}

func (m *awsIMDSModule) IssueName() string {
	return IssueName
}

func (m *awsIMDSModule) IssueType() string {
	return IssueType
}

func (m *awsIMDSModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil — the hop limit check runs once at startup, not periodically.
func (m *awsIMDSModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the AWS IMDS connectivity check once at agent startup.
func (m *awsIMDSModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return &runnerdef.BuiltInHealthCheck{
		Source: "core",
		Fn:     Check,
	}
}
