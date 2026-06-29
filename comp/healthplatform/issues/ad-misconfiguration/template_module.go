// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package admisconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

func init() {
	issues.RegisterModuleFactory(newTemplateModule)
}

const (
	// TemplateIssueName is the human-readable issue name for AD template resolution failure issues.
	TemplateIssueName = "Autodiscovery Template Resolution Error"
	// TemplateIssueID is the IssueID prefix for AD template resolution failure issues.
	// External reporters append name, service-id, and digest: TemplateIssueID + ":" + name + ":" + serviceID + ":" + digest
	TemplateIssueID = "ad-template"
)

type adTemplateModule struct {
	template *ADTemplateIssue
}

func newTemplateModule(config.Component) issues.Module {
	return &adTemplateModule{template: NewADTemplateIssue()}
}

func (m *adTemplateModule) IssueName() string {
	return TemplateIssueName
}

func (m *adTemplateModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

func (m *adTemplateModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

func (m *adTemplateModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
