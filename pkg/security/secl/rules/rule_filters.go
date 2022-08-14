// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/validators"
	"github.com/Masterminds/semver"
)

// RuleFilter definition of a rule filter
type RuleFilter interface {
	IsAccepted(rule *RuleDefinition, logger log.Logger) bool
}

// RuleIDFilter defines a ID based filter
type RuleIDFilter struct {
	ID string
}

// IsAccepted checks whether the rule is accepted
func (r *RuleIDFilter) IsAccepted(rule *RuleDefinition, _ log.Logger) bool {
	return r.ID == rule.ID
}

// AgentVersionFilter defines a agent version filter
type AgentVersionFilter struct {
	Version *semver.Version
}

func NewAgentVersionFilter(version *semver.Version) (*AgentVersionFilter, error) {
	withoutPreAgentVersion, err := version.SetPrerelease("")
	if err != nil {
		return nil, err
	}

	cleanAgentVersion, err := withoutPreAgentVersion.SetMetadata("")
	if err != nil {
		return nil, err
	}

	return &AgentVersionFilter{
		Version: &cleanAgentVersion,
	}, nil
}

// IsAccepted checks whether the rule is accepted
func (r *AgentVersionFilter) IsAccepted(rule *RuleDefinition, logger log.Logger) bool {
	withoutPreAgentVersion, err := r.Version.SetPrerelease("")
	if err != nil {
		return true
	}

	cleanAgentVersion, err := withoutPreAgentVersion.SetMetadata("")
	if err != nil {
		return true
	}

	constraint, err := validators.ValidateAgentVersionConstraint(rule.AgentVersionConstraint)
	if err != nil {
		logger.Errorf("failed to parse agent version constraint: %v", err)
		return false
	}

	return constraint.Check(&cleanAgentVersion)
}
