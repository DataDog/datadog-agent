// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package missedbytes reports log data the file tailer lost to rotation.
package missedbytes

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

const (
	// IssueName is the human-readable issue name for log data lost to rotation.
	IssueName = "Log Data Lost After Rotation"
	// IssueType is IssueName lowercased with spaces replaced by underscores.
	IssueType = "log_data_lost_after_rotation"
	// IssueID is the instance id prefix; the check appends a per-host digest.
	IssueID = "log-data-lost-after-rotation"
)

// checkSource must be unique: bundle.go only warns when Schedule rejects a dupe.
const checkSource = "logs-missed-bytes"

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type missedBytesModule struct {
	cfg     config.Component
	checker *checker
}

// NewModule creates the missed-bytes issue module.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &missedBytesModule{cfg: deps.Config, checker: newChecker(deps.Hostname)}
}

func (m *missedBytesModule) IssueName() string {
	return IssueName
}

func (m *missedBytesModule) IssueType() string {
	return IssueType
}

func (m *missedBytesModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return MissedBytesIssue{}.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck omits Interval to take the scheduler's 15-minute
// default; the tracker's window is 24 hours. The logs gate is inside Fn, not here,
// so a logs-disabled agent still resolves an issue a prior run left.
func (m *missedBytesModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: checkSource,
			Fn: func() ([]runnerdef.IssueReport, error) {
				if !logsEnabled(m.cfg) {
					return nil, nil
				}
				return m.checker.Run()
			},
		},
	}
}

// logsEnabled mirrors the logs agent's own gate, which still honours the
// deprecated log_enabled (comp/logs/agent/impl/agent.go).
func logsEnabled(cfg config.Component) bool {
	return cfg.GetBool("logs_enabled") || cfg.GetBool("log_enabled")
}

// BuiltInStartupHealthCheck returns nil: loss accrues while the agent runs.
func (m *missedBytesModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
