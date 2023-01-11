// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compliance defines common interfaces and types for Compliance Agent
package compliance

import "fmt"

// Rule defines an interface for rego and condition-fallback rules
type Rule interface {
	ResourceCount() int
	Common() *RuleCommon
}

// RuleCommon defines the base fields of a rule in a compliance config
type RuleCommon struct {
	ID          string        `yaml:"id"`
	Description string        `yaml:"description,omitempty"`
	Scope       RuleScopeList `yaml:"scope,omitempty"`
	SkipOnK8s   bool          `yaml:"skipOnKubernetes,omitempty"`
	Filters     []string      `yaml:"filters"`
}

// RegoRule defines a rule in a compliance config
type RegoRule struct {
	RuleCommon `yaml:",inline"`
	Inputs     []RegoInput `yaml:"input,omitempty"`
	Module     string      `yaml:"module,omitempty"`
	Imports    []string    `yaml:"imports,omitempty"`
	Findings   string      `yaml:"findings,omitempty"`
}

// HasResourceKind returns whether or the rule has a dependence on a least
// one resource with the given type.
func (r *RegoRule) HasResourceKind(resourceKind ResourceKind) bool {
	for _, input := range r.Inputs {
		if input.Kind() == resourceKind {
			return true
		}
	}
	return false
}

// ResourceCount returns the count of resources
func (r *RegoRule) ResourceCount() int {
	return len(r.Inputs)
}

// Common returns the common field between all rules
func (r *RegoRule) Common() *RuleCommon {
	return &r.RuleCommon
}

// RuleScope defines scope for applicability of a rule
type RuleScope string

const (
	// Host const
	Unscoped RuleScope = "none"
	// DockerScope const
	DockerScope RuleScope = "docker"
	// KubernetesNodeScope const
	KubernetesNodeScope RuleScope = "kubernetesNode"
	// KubernetesClusterScope const
	KubernetesClusterScope RuleScope = "kubernetesCluster"
)

// RuleScopeList is a set of RuleScopes
type RuleScopeList []RuleScope

// Includes returns true if RuleScopeList includes the specified RuleScope value
func (l RuleScopeList) Includes(ruleScope RuleScope) bool {
	for _, s := range l {
		if s == ruleScope {
			return true
		}
	}
	return false
}

// CheckName returns a canonical name of a check for a rule ID and description
func CheckName(ruleID string, description string) string {
	return fmt.Sprintf("%s: %s", ruleID, description)
}
