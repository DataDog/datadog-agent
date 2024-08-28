// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"time"
)

// MacroID represents the ID of a macro
type MacroID = string

// CombinePolicy represents the policy to use to combine rules and macros
type CombinePolicy = string

// Combine policies
const (
	NoPolicy       CombinePolicy = ""
	MergePolicy    CombinePolicy = "merge"
	OverridePolicy CombinePolicy = "override"
)

// OverrideField defines a combine field
type OverrideField = string

const (
	// OverrideAllFields used to override all the fields
	OverrideAllFields OverrideField = "all"
	// OverrideExpressionField used to override the expression
	OverrideExpressionField OverrideField = "expression"
	// OverrideActionFields used to override the actions
	OverrideActionFields OverrideField = "actions"
	// OverrideEveryField used to override the every field
	OverrideEveryField OverrideField = "every"
	// OverrideTagsField used to override the tags
	OverrideTagsField OverrideField = "tags"
)

// OverrideOptions defines combine options
type OverrideOptions struct {
	Fields []OverrideField `yaml:"fields"`
}

// MacroDefinition holds the definition of a macro
type MacroDefinition struct {
	ID                     MacroID       `yaml:"id"`
	Expression             string        `yaml:"expression"`
	Description            string        `yaml:"description"`
	AgentVersionConstraint string        `yaml:"agent_version"`
	Filters                []string      `yaml:"filters"`
	Values                 []string      `yaml:"values"`
	Combine                CombinePolicy `yaml:"combine"`
}

// RuleID represents the ID of a rule
type RuleID = string

// RuleDefinition holds the definition of a rule
type RuleDefinition struct {
	ID                     RuleID              `yaml:"id,omitempty"`
	Version                string              `yaml:"version,omitempty"`
	Expression             string              `yaml:"expression"`
	Description            string              `yaml:"description,omitempty"`
	Tags                   map[string]string   `yaml:"tags,omitempty"`
	AgentVersionConstraint string              `yaml:"agent_version,omitempty"`
	Filters                []string            `yaml:"filters,omitempty"`
	Disabled               bool                `yaml:"disabled,omitempty"`
	Combine                CombinePolicy       `yaml:"combine,omitempty"`
	OverrideOptions        OverrideOptions     `yaml:"override_options,omitempty"`
	Actions                []*ActionDefinition `yaml:"actions,omitempty"`
	Every                  time.Duration       `yaml:"every,omitempty"`
	Silent                 bool                `yaml:"silent,omitempty"`
	GroupID                string              `yaml:"group_id,omitempty"`
}

// GetTag returns the tag value associated with a tag key
func (rd *RuleDefinition) GetTag(tagKey string) (string, bool) {
	tagValue, ok := rd.Tags[tagKey]
	if ok {
		return tagValue, true
	}
	return "", false
}

// ActionName defines an action name
type ActionName = string

const (
	// KillAction name a the kill action
	KillAction ActionName = "kill"
)

// ActionDefinition describes a rule action section
type ActionDefinition struct {
	Filter   *string             `yaml:"filter"`
	Set      *SetDefinition      `yaml:"set"`
	Kill     *KillDefinition     `yaml:"kill"`
	CoreDump *CoreDumpDefinition `yaml:"coredump"`
	Hash     *HashDefinition     `yaml:"hash"`
}

// Scope describes the scope variables
type Scope string

// SetDefinition describes the 'set' section of a rule action
type SetDefinition struct {
	Name   string        `yaml:"name"`
	Value  interface{}   `yaml:"value"`
	Field  string        `yaml:"field"`
	Append bool          `yaml:"append"`
	Scope  Scope         `yaml:"scope"`
	Size   int           `yaml:"size"`
	TTL    time.Duration `yaml:"ttl"`
}

// KillDefinition describes the 'kill' section of a rule action
type KillDefinition struct {
	Signal string `yaml:"signal"`
	Scope  string `yaml:"scope"`
}

// CoreDumpDefinition describes the 'coredump' action
type CoreDumpDefinition struct {
	Process       bool `yaml:"process"`
	Mount         bool `yaml:"mount"`
	Dentry        bool `yaml:"dentry"`
	NoCompression bool `yaml:"no_compression"`
}

// HashDefinition describes the 'hash' section of a rule action
type HashDefinition struct{}

// OnDemandHookPoint represents a hook point definition
type OnDemandHookPoint struct {
	Name      string         `yaml:"name"`
	IsSyscall bool           `yaml:"syscall"`
	Args      []HookPointArg `yaml:"args"`
}

// HookPointArg represents the definition of a hook point argument
type HookPointArg struct {
	N    int    `yaml:"n"`
	Kind string `yaml:"kind"`
}

// PolicyDef represents a policy file definition
type PolicyDef struct {
	Version            string              `yaml:"version"`
	Macros             []*MacroDefinition  `yaml:"macros"`
	Rules              []*RuleDefinition   `yaml:"rules"`
	OnDemandHookPoints []OnDemandHookPoint `yaml:"hooks"`
}
