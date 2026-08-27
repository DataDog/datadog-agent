// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package missedbytes reports log data the file tailer lost to rotation through
// the Agent Health Platform.
package missedbytes

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

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
	// appends a per-(host, source, service) digest — see instanceIssueID.
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
	checker *checker
}

// NewModule creates the missed-bytes issue module, capturing the hostname the
// check needs to scope its issue ids; HealthCheckFunc takes no arguments.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &missedBytesModule{checker: newChecker(deps.Hostname)}
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
func (m *missedBytesModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: checkSource,
			Fn:     m.checker.Run,
		},
	}
}

// BuiltInStartupHealthCheck returns nil — loss accrues while the agent runs.
func (m *missedBytesModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
