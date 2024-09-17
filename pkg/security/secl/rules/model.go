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
	Fields []OverrideField `yaml:"fields" json:"fields" jsonschema:"enum=all,enum=expression,enum=actions,enum=every,enum=tags"`
}

// MacroDefinition holds the definition of a macro
type MacroDefinition struct {
	ID                     MacroID       `yaml:"id" json:"id"`
	Expression             string        `yaml:"expression" json:"expression,omitempty" jsonschema:"oneof_required=MacroWithExpression"`
	Description            string        `yaml:"description" json:"description,omitempty"`
	AgentVersionConstraint string        `yaml:"agent_version" json:"agent_version,omitempty"`
	Filters                []string      `yaml:"filters" json:"filters,omitempty"`
	Values                 []string      `yaml:"values" json:"values,omitempty" jsonschema:"oneof_required=MacroWithValues"`
	Combine                CombinePolicy `yaml:"combine" json:"combine,omitempty" jsonschema:"enum=merge,enum=override"`
}

// RuleID represents the ID of a rule
type RuleID = string

// RuleDefinition holds the definition of a rule
type RuleDefinition struct {
	ID                     RuleID              `yaml:"id" json:"id"`
	Version                string              `yaml:"version,omitempty" json:"version,omitempty"`
	Expression             string              `yaml:"expression" json:"expression,omitempty"`
	Description            string              `yaml:"description,omitempty" json:"description,omitempty"`
	Tags                   map[string]string   `yaml:"tags,omitempty" json:"tags,omitempty"`
	AgentVersionConstraint string              `yaml:"agent_version,omitempty" json:"agent_version,omitempty"`
	Filters                []string            `yaml:"filters,omitempty" json:"filters,omitempty"`
	Disabled               bool                `yaml:"disabled,omitempty" json:"disabled,omitempty"`
	Combine                CombinePolicy       `yaml:"combine,omitempty" json:"combine,omitempty" jsonschema:"enum=override"`
	OverrideOptions        OverrideOptions     `yaml:"override_options,omitempty" json:"override_options,omitempty"`
	Actions                []*ActionDefinition `yaml:"actions,omitempty" json:"actions,omitempty"`
	Every                  time.Duration       `yaml:"every,omitempty" json:"every,omitempty"`
	Silent                 bool                `yaml:"silent,omitempty" json:"silent,omitempty"`
	GroupID                string              `yaml:"group_id,omitempty" json:"group_id,omitempty"`
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
	// KillAction name of the kill action
	KillAction ActionName = "kill"
	// SetAction name of the set action
	SetAction ActionName = "set"
	// CoreDumpAction name of the core dump action
	CoreDumpAction ActionName = "coredump"
	// HashAction name of the hash action
	HashAction ActionName = "hash"
)

// ActionDefinition describes a rule action section
type ActionDefinition struct {
	Filter   *string             `yaml:"filter" json:"filter,omitempty"`
	Set      *SetDefinition      `yaml:"set" json:"set,omitempty" jsonschema:"oneof_required=SetAction"`
	Kill     *KillDefinition     `yaml:"kill" json:"kill,omitempty" jsonschema:"oneof_required=KillAction"`
	CoreDump *CoreDumpDefinition `yaml:"coredump" json:"coredump,omitempty" jsonschema:"oneof_required=CoreDumpAction"`
	Hash     *HashDefinition     `yaml:"hash" json:"hash,omitempty" jsonschema:"oneof_required=HashAction"`
}

// Name returns the name of the action
func (a *ActionDefinition) Name() ActionName {
	switch {
	case a.Set != nil:
		return SetAction
	case a.Kill != nil:
		return KillAction
	case a.CoreDump != nil:
		return CoreDumpAction
	case a.Hash != nil:
		return HashAction
	default:
		return ""
	}
}

// Scope describes the scope variables
type Scope string

// SetDefinition describes the 'set' section of a rule action
type SetDefinition struct {
	Name   string        `yaml:"name" json:"name"`
	Value  interface{}   `yaml:"value" json:"value,omitempty" jsonschema:"oneof_required=SetWithValue"`
	Field  string        `yaml:"field" json:"field,omitempty" jsonschema:"oneof_required=SetWithField"`
	Append bool          `yaml:"append" json:"append,omitempty"`
	Scope  Scope         `yaml:"scope" json:"scope,omitempty" jsonschema:"enum=process,enum=container"`
	Size   int           `yaml:"size" json:"size,omitempty"`
	TTL    time.Duration `yaml:"ttl" json:"ttl,omitempty"`
}

// KillDefinition describes the 'kill' section of a rule action
type KillDefinition struct {
	Signal string `yaml:"signal" json:"signal" jsonschema:"description=A valid signal name,example=SIGKILL,example=SIGTERM"`
	Scope  string `yaml:"scope" json:"scope,omitempty" jsonschema:"enum=process,enum=container"`
}

// CoreDumpDefinition describes the 'coredump' action
type CoreDumpDefinition struct {
	Process       bool `yaml:"process" json:"process,omitempty" jsonschema:"anyof_required=CoreDumpWithProcess"`
	Mount         bool `yaml:"mount" json:"mount,omitempty" jsonschema:"anyof_required=CoreDumpWithMount"`
	Dentry        bool `yaml:"dentry" json:"dentry,omitempty" jsonschema:"anyof_required=CoreDumpWithDentry"`
	NoCompression bool `yaml:"no_compression" json:"no_compression,omitempty"`
}

// HashDefinition describes the 'hash' section of a rule action
type HashDefinition struct{}

// OnDemandHookPoint represents a hook point definition
type OnDemandHookPoint struct {
	Name      string         `yaml:"name" json:"name"`
	IsSyscall bool           `yaml:"syscall" json:"syscall,omitempty"`
	Args      []HookPointArg `yaml:"args" json:"args,omitempty"`
}

// HookPointArg represents the definition of a hook point argument
type HookPointArg struct {
	N    int    `yaml:"n" json:"n" jsonschema:"description=Zero-based argument index"`
	Kind string `yaml:"kind" json:"kind" jsonschema:"enum=uint,enum=null-terminated-string"`
}

// PolicyDef represents a policy file definition
type PolicyDef struct {
	Version            string              `yaml:"version" json:"version"`
	Macros             []*MacroDefinition  `yaml:"macros" json:"macros,omitempty"`
	Rules              []*RuleDefinition   `yaml:"rules" json:"rules"`
	OnDemandHookPoints []OnDemandHookPoint `yaml:"hooks" json:"hooks,omitempty"`
}
