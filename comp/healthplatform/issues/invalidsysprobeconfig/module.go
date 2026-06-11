// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidsysprobeconfig reports system-probe.yaml schema violations through the Agent Health Platform.
package invalidsysprobeconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/schemacheck"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
)

// IssueID is the stable Agent Health identifier for system-probe configuration-schema violations
const IssueID = "invalid-system-probe-config"

var check = schemacheck.Check{
	IssueID:            IssueID,
	Validator:          schema.ValidateSystemProbeConfig,
	Subject:            "Datadog system-probe configuration",
	ViolationNoun:      "system-probe schema",
	Location:           "system-probe",
	Tags:               []string{"config", "schema", "system-probe"},
	Impact:             "The Datadog system-probe may apply defaults for incorrectly-typed fields and may not behave as configured.",
	RemediationSummary: "Fix each schema violation in the system-probe configuration, then restart the Datadog Agent.",
}

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type invalidSysprobeConfigModule struct {
	cfg sysprobeconfig.Component
}

// NewModule captures the system-probe config so the once-only startup check can read it.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &invalidSysprobeConfigModule{cfg: deps.SysProbeConfig}
}

func (m *invalidSysprobeConfigModule) IssueName() string { return IssueID }

func (m *invalidSysprobeConfigModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return check.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil as schema validation runs only at startup
func (m *invalidSysprobeConfigModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the system-probe schema validation once at agent startup.
func (m *invalidSysprobeConfigModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	if m.cfg == nil {
		// sysprobeconfig isn't bundled
		return nil
	}
	return &runnerdef.BuiltInHealthCheck{
		Source: "system-probe",
		Fn: func() ([]runnerdef.IssueReport, error) {
			return check.Run(m.cfg)
		},
	}
}
