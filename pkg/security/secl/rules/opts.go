// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// VariableProvider is the interface implemented by SECL variable providers
// (Should be named VariableValueProvider)
type VariableProvider interface {
	NewSECLVariable(name string, value interface{}, opts eval.VariableOpts) (eval.SECLVariable, error)
	CleanupExpiredVariables()
}

// VariableProviderFactory describes a function called to instantiate a variable provider
type VariableProviderFactory func() VariableProvider

// RuleActionPerformedCb describes the callback function called after a rule action is performed
type RuleActionPerformedCb func(r *Rule, action *ActionDefinition)

// Opts defines rules set options
type Opts struct {
	SupportedDiscarders        map[eval.Field]bool
	SupportedMultiDiscarders   []*MultiDiscarder
	ExcludedRuleFromDiscarders map[eval.RuleID]bool
	ReservedRuleIDs            []RuleID
	EventTypeEnabled           map[eval.EventType]bool
	StateScopes                map[Scope]VariableProviderFactory
	Logger                     log.Logger
	ruleActionPerformedCb      RuleActionPerformedCb
}

// WithSupportedDiscarders set supported discarders
func (o *Opts) WithSupportedDiscarders(discarders map[eval.Field]bool) *Opts {
	o.SupportedDiscarders = discarders
	return o
}

// WithSupportedMultiDiscarder set supported multi discarders
func (o *Opts) WithSupportedMultiDiscarder(discarders []*MultiDiscarder) *Opts {
	o.SupportedMultiDiscarders = discarders
	return o
}

// WithExcludedRuleFromDiscarders set excluded rule from discarders
func (o *Opts) WithExcludedRuleFromDiscarders(excludedRuleFromDiscarders map[eval.RuleID]bool) *Opts {
	o.ExcludedRuleFromDiscarders = excludedRuleFromDiscarders
	return o
}

// WithEventTypeEnabled set event types enabled
func (o *Opts) WithEventTypeEnabled(eventTypes map[eval.EventType]bool) *Opts {
	o.EventTypeEnabled = eventTypes
	return o
}

// WithReservedRuleIDs set reserved rule ids
func (o *Opts) WithReservedRuleIDs(ruleIDs []RuleID) *Opts {
	o.ReservedRuleIDs = ruleIDs
	return o
}

// WithLogger set logger
func (o *Opts) WithLogger(logger log.Logger) *Opts {
	o.Logger = logger
	return o
}

// WithStateScopes set state scopes
func (o *Opts) WithStateScopes(stateScopes map[Scope]VariableProviderFactory) *Opts {
	o.StateScopes = stateScopes
	return o
}

// WithRuleActionPerformedCb sets the rule action performed callback
func (o *Opts) WithRuleActionPerformedCb(cb RuleActionPerformedCb) *Opts {
	o.ruleActionPerformedCb = cb
	return o
}

// NewRuleOpts returns rule options
func NewRuleOpts(eventTypeEnabled map[eval.EventType]bool) *Opts {
	var ruleOpts Opts
	ruleOpts.
		WithEventTypeEnabled(eventTypeEnabled).
		WithStateScopes(DefaultStateScopes())

	return &ruleOpts
}

// NewEvalOpts returns eval options
func NewEvalOpts() *eval.Opts {
	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants()).
		WithLegacyFields(model.SECLLegacyFields).
		WithVariables(model.SECLVariables)

	return &evalOpts
}

// NewBothOpts returns rule and eval options
func NewBothOpts(eventTypeEnabled map[eval.EventType]bool) (*Opts, *eval.Opts) {
	return NewRuleOpts(eventTypeEnabled), NewEvalOpts()
}

// MultiDiscarder represents a multi discarder, i.e. a discarder across multiple rule buckets
type MultiDiscarder struct {
	Entries        []MultiDiscarderEntry
	FinalField     string
	FinalEventType model.EventType
}

// MultiDiscarderEntry represents a multi discarder entry (a field, and associated event type)
type MultiDiscarderEntry struct {
	Field     string
	EventType model.EventType
}
