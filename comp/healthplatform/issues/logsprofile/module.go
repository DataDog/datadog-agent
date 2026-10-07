// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsprofile recommends a logs performance profile when the logs pipeline is saturated.
package logsprofile

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

const (
	// IssueName is the issue name used while logs are being lost.
	IssueName = "Logs Performance Profile Recommended"
	// IssueType is IssueName lowercased with spaces replaced by underscores.
	IssueType = "logs_performance_profile_recommended"
	// IssueID is the instance id prefix; the check appends a per-host digest.
	IssueID = "logs-performance-profile-recommended"

	// SuggestedIssueName is the issue name used while the pipeline is saturated but lossless.
	SuggestedIssueName = "Logs Performance Profile Suggested"
	// SuggestedIssueType is SuggestedIssueName lowercased with spaces replaced by underscores.
	SuggestedIssueType = "logs_performance_profile_suggested"
	// SuggestedIssueID is the instance id prefix; the check appends a per-host digest.
	SuggestedIssueID = "logs-performance-profile-suggested"
)

// checkSource must be unique: bundle.go only warns when Schedule rejects a dupe.
const checkSource = "logs-profile-recommendation"

func init() {
	issues.RegisterModuleFactory(NewModule)
	issues.RegisterModuleFactory(NewSuggestedModule)
}

type recommendedModule struct {
	deps    issues.ModuleDeps
	checker *checker
}

// NewModule creates the module that owns the profile check and the "recommended" template.
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &recommendedModule{deps: deps, checker: newChecker(deps.Config, deps.Hostname)}
}

func (m *recommendedModule) IssueName() string {
	return IssueName
}

func (m *recommendedModule) IssueType() string {
	return IssueType
}

func (m *recommendedModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return RecommendedIssue{}.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck pre-seeds SuggestedIssueName so restart resolution covers both names.
func (m *recommendedModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source:     checkSource,
			Fn:         m.checker.Run,
			IssueNames: []string{SuggestedIssueName},
		},
		Interval: m.deps.Config.GetDuration(cfgInterval),
	}
}

// BuiltInStartupHealthCheck returns nil: saturation accrues while the agent runs.
func (m *recommendedModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}

type suggestedModule struct{}

// NewSuggestedModule registers the "suggested" template; the shared check reports under it.
func NewSuggestedModule(issues.ModuleDeps) issues.Module {
	return suggestedModule{}
}

func (suggestedModule) IssueName() string {
	return SuggestedIssueName
}

func (suggestedModule) IssueType() string {
	return SuggestedIssueType
}

func (suggestedModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return SuggestedIssue{}.BuildIssue(context)
}

func (suggestedModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

func (suggestedModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}
