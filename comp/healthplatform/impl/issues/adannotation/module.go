// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package adannotation provides a complete module for handling AD annotation issues
package adannotation

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	IssueID = "misconfigured-ad-annotation"
)

type adAnnotationModule struct {
	template *ADAnnotationIssue
	conf     config.Component
}

func NewModule(config config.Component) issues.Module {
	return &adAnnotationModule{
		template: NewADAnnotationIssue(),
		conf:     config,
	}
}

func (a *adAnnotationModule) IssueID() string {
	return IssueID
}

func (a *adAnnotationModule) IssueTemplate() issues.IssueTemplate {
	return a.template
}

func (a *adAnnotationModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
