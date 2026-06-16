// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package invalidconfig reports datadog.yaml schema violations through the Agent Health Platform.
package invalidconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/schemacheck"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/config/schema"
)

// IssueID is the stable Agent Health identifier for configuration-schema violations
const IssueID = "invalid-config"

var check = schemacheck.Check{
	IssueID:            IssueID,
	Validator:          schema.ValidateCoreConfig,
	Subject:            "Datadog Agent configuration",
	ViolationNoun:      "schema",
	Location:           "agent",
	Tags:               []string{"config", "schema"},
	Impact:             "The Datadog Agent may apply defaults for incorrectly-typed fields and may not behave as configured.",
	RemediationSummary: "Fix each schema violation in the configuration file, then restart the Datadog Agent.",
}

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type invalidConfigModule struct {
	cfg config.Component
}

// NewModule captures the config so the once-only startup check can read it.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &invalidConfigModule{cfg: deps.Config}
}

func (m *invalidConfigModule) IssueName() string { return IssueID }

func (m *invalidConfigModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return check.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil as schema validation runs only at startup
func (m *invalidConfigModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs schema validation once at agent startup.
// The check is gated inside Fn rather than at registration time so that
// IssueNames-based stale-issue resolution still fires on restart even when
// the flag is disabled — returning nil/empty resolves any previously-stored
// issues rather than leaving them orphaned.
//
// The gate exists because schema.ValidateCoreConfig decompresses, parses, and
// compiles the full core_schema.yaml (~8000 lines) into a *jsonschema.Schema
// stored in a process-lifetime global — adding ~8 MiB of permanent heap even
// when the agent config is valid.
func (m *invalidConfigModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return &runnerdef.BuiltInHealthCheck{
		Source: "agent",
		Fn: func() ([]runnerdef.IssueReport, error) {
			if !m.cfg.GetBool("health_platform.invalidconfig_check.enabled") {
				return nil, nil
			}
			return check.Run(m.cfg)
		},
	}
}
