// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

// AgentContext represents the context of an agent
type AgentContext struct {
	RuleID string `json:"rule_id" mapstructure:"rule_id"`
	Origin string `json:"origin" mapstructure:"origin"`
}

// Event represents a cws event
type Event struct {
	Agent AgentContext `json:"agent" mapstructure:"agent"`
}

// Evt contains information about a rule event
type Evt struct {
	Name string `json:"name" mapstructure:"name"`
}

// RuleEvent represents a rule event
type RuleEvent struct {
	Event   `mapstructure:",squash"`
	Evt     `json:"evt" mapstructure:"evt"`
	Process Process `json:"process" mapstructure:"process"`
	File    File    `json:"file" mapstructure:"file"`
}

// File represents a file
type File struct {
	Path string `json:"path" mapstructure:"path"`
}

// Process represents a process
type Process struct {
	Executable File `json:"executable" mapstructure:"executable"`
}

// Policy represents a policy
type Policy struct {
	Name   string `json:"name" mapstructure:"name"`
	Source string `json:"source" mapstructure:"source"`
}

// RulesetLoadedEvent represents a ruleset loaded event
type RulesetLoadedEvent struct {
	Event    `mapstructure:",squash"`
	Policies []Policy `json:"policies" mapstructure:"policies"`
}

// ContainsPolicy checks if a policy, given its source and name, is contained in the ruleset loaded event
func (e *RulesetLoadedEvent) ContainsPolicy(policySource string, policyName string) bool {
	for _, policy := range e.Policies {
		if policy.Source == policySource && policy.Name == policyName {
			return true
		}
	}
	return false
}

// SelftestsEvent represents a selftests event
type SelftestsEvent struct {
	Event          `mapstructure:",squash"`
	SucceededTests []string `json:"succeeded_tests" mapstructure:"succeeded_tests"`
	FailedTests    []string `json:"failed_tests" mapstructure:"failed_tests"`
}
