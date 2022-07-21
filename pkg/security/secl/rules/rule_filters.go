// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"strings"

	"github.com/Masterminds/semver"
)

// RuleFilter definition of a rule filter
type RuleFilter interface {
	IsAccepted(rule *RuleDefinition) bool
}

// RuleIDFilter defines a ID based filter
type RuleIDFilter struct {
	ID string
}

// IsAccepted checks whether the rule is accepted
func (r *RuleIDFilter) IsAccepted(rule *RuleDefinition) bool {
	return r.ID == rule.ID
}

// AgentVersionFilter defines a agent version filter
type AgentVersionFilter struct {
	Version *semver.Version
}

// IsAccepted checks whether the rule is accepted
func (r *AgentVersionFilter) IsAccepted(rule *RuleDefinition) bool {
	withoutPreAgentVersion, err := r.Version.SetPrerelease("")
	if err != nil {
		return true
	}

	cleanAgentVersion, err := withoutPreAgentVersion.SetMetadata("")
	if err != nil {
		return true
	}

	constraint := strings.TrimSpace(rule.AgentVersionConstraint)
	if constraint == "" {
		return true
	}

	semverConstraint, err := semver.NewConstraint(constraint)
	if err != nil {
		return false
	}

	return semverConstraint.Check(&cleanAgentVersion)
}
