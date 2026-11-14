// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsdropped reports logs the Agent read but dropped before delivering
// them: rejected by the intake, failed for good on a destination, or unable to be
// batched. Loss before the Agent reads a file is missedbytes' job.
package logsdropped

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

const (
	// IssueName is the human-readable issue name for logs dropped before delivery.
	IssueName = "Logs Dropped Before Delivery"
	// IssueType is IssueName lowercased with spaces replaced by underscores.
	IssueType = "logs_dropped_before_delivery"
	// IssueID is the instance id prefix; the check appends a per-host digest.
	IssueID = "logs-dropped-before-delivery"
)

// checkSource must be unique: bundle.go only warns when Schedule rejects a dupe.
const checkSource = "logs-dropped"

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type logsDroppedModule struct {
	cfg     config.Component
	checker *checker
}

// NewModule creates the dropped-logs issue module.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &logsDroppedModule{cfg: deps.Config, checker: newChecker(deps.Hostname)}
}

func (m *logsDroppedModule) IssueName() string {
	return IssueName
}

func (m *logsDroppedModule) IssueType() string {
	return IssueType
}

func (m *logsDroppedModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return LogsDroppedIssue{}.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck omits Interval to take the scheduler's 15-minute
// default; the tracker's window is 24 hours. The logs gate is inside Fn, not here,
// so a logs-disabled agent still resolves an issue a prior run left.
func (m *logsDroppedModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: checkSource,
			Fn: func() ([]runnerdef.IssueReport, error) {
				if !logsissue.LogsEnabled(m.cfg) {
					return nil, nil
				}
				return m.checker.Run()
			},
		},
	}
}

// BuiltInStartupHealthCheck returns nil: drops accrue while the agent runs.
func (m *logsDroppedModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
