// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package awsimds detects when a containerized agent on AWS EC2 cannot reach IMDS due to the default hop limit of 1.
package awsimds

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"net/url"

	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
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
	// IssueName is the template registry key and proto IssueName field.
	IssueName = "AWS IMDS Hop Limit"

	// IssueType is IssueName lowercased with spaces replaced by underscores.
	IssueType = "aws_imds_hop_limit"

	// IssueID is the kebab-case instance id prefix used when reporting this issue.
	IssueID = "aws-imds-hop-limit"
)

// awsIMDSModule implements issues.Module
type awsIMDSModule struct {
	template *AWSIMDSIssue
	hostname hostnameinterface.Component
	cfg      config.Component
}

// NewModule returns the module, or nil to decline registration when not a container on AWS.
func NewModule(deps issues.ModuleDeps) issues.Module {
	if !env.IsContainerized() || !ec2.IsRunningOnFromDMI() {
		return nil
	}
	return newModule(deps)
}

func newModule(deps issues.ModuleDeps) *awsIMDSModule {
	return &awsIMDSModule{
		template: NewAWSIMDSIssue(),
		hostname: deps.Hostname,
		cfg:      deps.Config,
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
	// Skip when AWS metadata collection is intentionally disabled, so we don't nag to re-enable it.
	if m.cfg != nil && !configutils.IsCloudProviderEnabled(ec2.CloudProviderName, m.cfg) {
		return nil, nil
	}

	// Environment is gated at registration; a clean probe here resolves any stored issue.
	detected, err := probe()
	if err != nil || !detected {
		return nil, err
	}
	hostnameConfigured := "false"
	if m.cfg != nil && m.cfg.GetString("hostname") != "" {
		hostnameConfigured = "true"
	}
	return []runnerdef.IssueReport{{
		IssueID:   m.instanceIssueID(),
		IssueName: IssueName,
		Context: map[string]string{
			contextKeyIMDSAddress:        imdsAddress,
			contextKeyHostnameConfigured: hostnameConfigured,
		},
		Tags: []string{"aws", "imds", "hop-limit", "container"},
	}}, nil
}

// instanceIssueID scopes the issue per host, since each EC2 instance's hop limit is configured independently.
func (m *awsIMDSModule) instanceIssueID() string {
	h := fnv.New64a()
	h.Write([]byte(m.hostname.GetSafe(context.Background())))
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
}
