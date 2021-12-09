// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// Opts defines rules set options
type Opts struct {
	eval.Opts
	SupportedDiscarders map[eval.Field]bool
	ReservedRuleIDs     []RuleID
	EventTypeEnabled    map[eval.EventType]bool
	Logger              Logger
}

// WithConstants set constants
func (o *Opts) WithConstants(constants map[string]interface{}) *Opts {
	o.Opts.WithConstants(constants)
	return o
}

// WithVariables set variables
func (o *Opts) WithVariables(variables map[string]eval.VariableValue) *Opts {
	o.Opts.WithVariables(variables)
	return o
}

// WithLegacyFields set legacy fields
func (o *Opts) WithLegacyFields(fields map[eval.Field]eval.Field) *Opts {
	o.Opts.WithLegacyFields(fields)
	return o
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
