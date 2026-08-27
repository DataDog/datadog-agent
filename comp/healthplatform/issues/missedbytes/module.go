// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package missedbytes reports log data the file tailer lost to rotation through
// the Agent Health Platform.
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
	// IssueType is the snake_case type key for log data lost to rotation:
	// IssueName lowercased with spaces replaced by underscores.
	IssueType = "log_data_lost_after_rotation"
	// IssueID is the stable instance identifier prefix (kebab-case). The check
	// appends a per-host digest — see hostIssueID.
	IssueID = "log-data-lost-after-rotation"
)

// checkSource is this module's scheduler key. It must be unique across modules:
// Schedule rejects a duplicate and bundle.go only warns, so a collision would
// silently never schedule the check.
const checkSource = "logs-missed-bytes"

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type missedBytesModule struct {
	cfg     config.Component
	checker *checker
}

// NewModule creates the missed-bytes issue module, capturing the hostname the
// check needs to scope its issue ids; HealthCheckFunc takes no arguments.
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

// BuiltInPeriodicHealthCheck returns the periodic health check configuration.
// Interval is omitted so the scheduler's default (15 minutes) applies; the
// tracker's window is 24 hours, so polling faster adds no signal.
//
// The logs_enabled gate lives inside Fn rather than returning nil here, so that
// an agent whose logs were turned off still resolves an issue a previous
// logs-enabled run left in the store. It also keeps the check quiet on the
// large share of the fleet that runs with logs_enabled false: health_platform
// is on by default and logs are not, and checker.Run's inactive case reports an
// error, which the runner and the scheduler each log at warn on every tick.
func (m *missedBytesModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: checkSource,
			Fn: func() ([]runnerdef.IssueReport, error) {
				if !m.cfg.GetBool("logs_enabled") {
					return nil, nil
				}
				return m.checker.Run()
			},
		},
	}
}

// BuiltInStartupHealthCheck returns nil — loss accrues while the agent runs.
func (m *missedBytesModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
