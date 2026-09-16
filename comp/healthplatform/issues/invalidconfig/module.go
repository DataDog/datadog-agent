// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

const (
	// IssueName is the human-readable issue name for configuration-schema violations.
	IssueName = "Invalid Config"
	// IssueType is the snake_case type key for configuration-schema violations:
	// IssueName lowercased with spaces replaced by underscores.
	IssueType = "invalid_config"
	// IssueID is the stable instance identifier / registry key (kebab-case).
	IssueID = "invalid-config"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type invalidConfigModule struct {
	cfg     config.Component
	checker *checker
}

// NewModule captures the config so the once-only startup check can read it.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &invalidConfigModule{cfg: deps.Config, checker: newChecker(deps.Config, deps.Hostname, deps.SelfIdent)}
}

func (m *invalidConfigModule) IssueName() string {
	return IssueName
}

func (m *invalidConfigModule) IssueType() string {
	return IssueType
}

func (m *invalidConfigModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return InvalidConfigIssue{}.BuildIssue(context)
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
			return m.checker.Run()
		},
	}
}
