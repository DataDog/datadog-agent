// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package admisconfig provides an issue module for autodiscovery misconfigurations.
// This module only provides remediation (no built-in check) as annotation errors
// are reported by external integrations (the container config provider).
package admisconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

// adMisconfigurationModule implements issues.Module
type adMisconfigurationModule struct {
	template *ADMisconfigurationIssue
}

// NewModule creates a new AD annotation issue module
func NewModule(config.Component) issues.Module {
	return &adMisconfigurationModule{
		template: NewADMisconfigurationIssue(),
	}
}

func (m *adMisconfigurationModule) IssueName() string {
	return storedef.ADMisconfigurationIssueName
}

func (m *adMisconfigurationModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return m.template.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil - annotation errors are reported by the container config provider
func (m *adMisconfigurationModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck returns nil - no startup-time check for this module
func (m *adMisconfigurationModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
