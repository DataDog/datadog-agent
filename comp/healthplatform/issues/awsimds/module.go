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
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"net/url"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
)

// imdsAddress can be overridden by tests to dial a local listener.
var imdsAddress = func() string {
	endpoint, err := url.Parse(ec2.TokenURL)
	if err != nil {
		panic(err)
	}
	return net.JoinHostPort(endpoint.Hostname(), "80")
}()

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
	template  *AWSIMDSIssue
	hostname  hostnameinterface.Component
	selfIdent *selfident.SelfIdent
}

// NewModule creates a new AWS IMDS hop limit issue module, or nil to decline
// registration entirely when the agent is not in a container on AWS. This gate
// is on the environment (immutable for a given host), not on config, so there
// is no stale-issue-resolution reason to register a module that can never fire.
func NewModule(deps issues.ModuleDeps) issues.Module {
	if !env.IsContainerized() || !ec2.IsRunningOnFromDMI() {
		return nil
	}
	return newModule(deps)
}

func newModule(deps issues.ModuleDeps) *awsIMDSModule {
	return &awsIMDSModule{
		template:  NewAWSIMDSIssue(),
		hostname:  deps.Hostname,
		selfIdent: deps.SelfIdent,
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
		Fn:     m.check,
	}
}

func (m *awsIMDSModule) check() ([]runnerdef.IssueReport, error) {
	// Environment is already gated at registration (NewModule); only the probe
	// result varies here, so a clean probe resolves any previously stored issue.
	detected, err := probe()
	if err != nil || !detected {
		return nil, err
	}
	return []runnerdef.IssueReport{{
		IssueID:   m.instanceIssueID(),
		IssueName: IssueName,
		Context: map[string]string{
			contextKeyIMDSAddress: imdsAddress,
		},
		Tags: []string{"aws", "imds", "hop-limit", "container"},
	}}, nil
}

// instanceIssueID scopes the issue to this agent's discriminator.
func (m *awsIMDSModule) instanceIssueID() string {
	h := fnv.New64a()
	discriminator := issues.IssueDiscriminator(m.selfIdent, m.hostname.GetSafe(context.Background()))
	fmt.Fprint(h, discriminator)
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
}
