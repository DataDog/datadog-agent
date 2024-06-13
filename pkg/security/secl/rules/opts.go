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
type VariableProvider interface {
	GetVariable(name string, value interface{}, opts eval.VariableOpts) (eval.VariableValue, error)
}

// VariableProviderFactory describes a function called to instantiate a variable provider
type VariableProviderFactory func() VariableProvider

// Opts defines rules set options
type Opts struct {
	RuleSetTag               map[string]eval.RuleSetTagValue
	SupportedDiscarders      map[eval.Field]bool
	SupportedMultiDiscarders []*MultiDiscarder
	ReservedRuleIDs          []RuleID
	EventTypeEnabled         map[eval.EventType]bool
	StateScopes              map[Scope]VariableProviderFactory
	Logger                   log.Logger
}

// WithRuleSetTag sets the rule set tag with the value of the tag of the rules that belong in this rule set
func (o *Opts) WithRuleSetTag(tagValue eval.RuleSetTagValue) *Opts {
	if o.RuleSetTag == nil {
		o.RuleSetTag = make(map[string]eval.RuleSetTagValue)
	}
	o.RuleSetTag[RuleSetTagKey] = tagValue
	return o
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

// WithEventTypeEnabled set event types enabled
func (o *Opts) WithEventTypeEnabled(eventTypes map[eval.EventType]bool) *Opts {
	o.EventTypeEnabled = eventTypes
	return o
}

// WithReservedRuleIDs set reserved rule ids
func (o *Opts) WithReservedRuleIDs(ruleIds []RuleID) *Opts {
	o.ReservedRuleIDs = ruleIds
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

// NewRuleOpts returns rule options
func NewRuleOpts(eventTypeEnabled map[eval.EventType]bool) *Opts {
	var ruleOpts Opts
	ruleOpts.
		WithEventTypeEnabled(eventTypeEnabled).
		WithStateScopes(map[Scope]VariableProviderFactory{
			"process": func() VariableProvider {
				return eval.NewScopedVariables(func(ctx *eval.Context) eval.ScopedVariable {
					return ctx.Event.(*model.Event).ProcessCacheEntry
				})
			},
			"container": func() VariableProvider {
				return eval.NewScopedVariables(func(ctx *eval.Context) eval.ScopedVariable {
					return ctx.Event.(*model.Event).ContainerContext
				})
			},
		}).WithRuleSetTag(DefaultRuleSetTagValue)

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
