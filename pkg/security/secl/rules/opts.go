// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// VariableProvider is the interface implemented by SECL variable providers
type VariableProvider interface {
	GetVariable(name string, value interface{}) (eval.VariableValue, error)
}

// VariableProviderFactory describes a function called to instantiate a variable provider
type VariableProviderFactory func() VariableProvider

// Opts defines rules set options
type Opts struct {
	SupportedDiscarders map[eval.Field]bool
	ReservedRuleIDs     []RuleID
	EventTypeEnabled    map[eval.EventType]bool
	StateScopes         map[Scope]VariableProviderFactory
	Logger              Logger
}

// WithSupportedDiscarders set supported discarders
func (o *Opts) WithSupportedDiscarders(discarders map[eval.Field]bool) *Opts {
	o.SupportedDiscarders = discarders
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
func (o *Opts) WithLogger(logger Logger) *Opts {
	o.Logger = logger
	return o
}

// WithStateScopes set state scopes
func (o *Opts) WithStateScopes(stateScopes map[Scope]VariableProviderFactory) *Opts {
	o.StateScopes = stateScopes
	return o
}
