// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package untailedfiles reports log files the Agent matched but does not tail
// because logs_config.open_files_limit was reached. Logs written to them are not
// collected, and are lost if the files rotate away before a slot frees up.
package untailedfiles

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/internal/logsissue"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

const (
	// IssueName is the human-readable issue name for log files left untailed.
	IssueName = "Log Files Not Tailed"
	// IssueType is IssueName lowercased with spaces replaced by underscores.
	IssueType = "log_files_not_tailed"
	// IssueID is the instance id prefix; the check appends a per-host digest.
	IssueID = "log-files-not-tailed"
)

// checkSource must be unique: bundle.go only warns when Schedule rejects a dupe.
const checkSource = "logs-untailed-files"

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type untailedFilesModule struct {
	cfg     config.Component
	checker *checker
}

// NewModule creates the untailed-files issue module.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &untailedFilesModule{cfg: deps.Config, checker: newChecker(deps.Hostname)}
}

func (m *untailedFilesModule) IssueName() string {
	return IssueName
}

func (m *untailedFilesModule) IssueType() string {
	return IssueType
}

func (m *untailedFilesModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return UntailedFilesIssue{}.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck omits Interval to take the scheduler's 15-minute
// default; the tracker holds a hit for an hour. The logs gate is inside Fn, not
// here, so a logs-disabled agent still resolves an issue a prior run left.
func (m *untailedFilesModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
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

// BuiltInStartupHealthCheck returns nil: scans hit the limit while the agent runs.
func (m *untailedFilesModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
