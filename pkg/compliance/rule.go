// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package compliance defines common interfaces and types for Compliance Agent
package compliance

import "fmt"

// Rule defines a rule in a compliance config
type Rule struct {
	ID           string        `yaml:"id"`
	Description  string        `yaml:"description,omitempty"`
	Scope        RuleScopeList `yaml:"scope,omitempty"`
	HostSelector string        `yaml:"hostSelector,omitempty"`
	Resources    []Resource    `yaml:"resources,omitempty"`
}

// RuleScope defines scope for applicability of a rule
type RuleScope string

const (
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
