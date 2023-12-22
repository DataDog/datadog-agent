// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrRuleWithoutID is returned when there is no ID
	ErrRuleWithoutID = errors.New("no rule ID")

	// ErrRuleWithoutExpression is returned when there is no expression
	ErrRuleWithoutExpression = errors.New("no rule expression")

	// ErrRuleIDPattern is returned when there is no expression
	ErrRuleIDPattern = errors.New("rule ID pattern error")

	// ErrRuleWithoutEvent is returned when no event type was inferred from the rule
	ErrRuleWithoutEvent = errors.New("no event in the rule definition")

	// ErrRuleWithMultipleEvents is returned when multiple event type were inferred from the rule
	ErrRuleWithMultipleEvents = errors.New("rule with multiple events is not supported")

	// ErrDefinitionIDConflict is returned when multiple rules use the same ID
	ErrDefinitionIDConflict = errors.New("multiple definition with the same ID")

	// ErrInternalIDConflict is returned when a user defined rule use an internal ID
	ErrInternalIDConflict = errors.New("internal rule ID conflict")

	// ErrEventTypeNotEnabled is returned when an event is not enabled
	ErrEventTypeNotEnabled = errors.New("event type not enabled")

	// ErrCannotMergeExpression is returned when trying to merge SECL expression
	ErrCannotMergeExpression = errors.New("cannot merge expression")

	// ErrRuleAgentVersion is returned when there is an agent version error
	ErrRuleAgentVersion = errors.New("agent version incompatible")

	// ErrRuleAgentFilter is returned when an agent rule was filtered
	ErrRuleAgentFilter = errors.New("agent rule filtered")

	// ErrNoRuleSetsInEvaluationSet is returned when no rule sets were provided to instantiate an evaluation set
	ErrNoRuleSetsInEvaluationSet = errors.New("no rule sets provided to instantiate an evaluation set")

	// ErrCannotChangeTagAfterLoading is returned when an attempt was made to change the tag on a ruleset that already has rules loaded
	ErrCannotChangeTagAfterLoading = errors.New("cannot change tag on a rule set that already has rules loaded")
)

// ErrFieldTypeUnknown is returned when a field has an unknown type
type ErrFieldTypeUnknown struct {
	Field string
}

func (e *ErrFieldTypeUnknown) Error() string {
	return fmt.Sprintf("field type unknown for `%s`", e.Field)
}

// ErrValueTypeUnknown is returned when the value of a field has an unknown type
type ErrValueTypeUnknown struct {
	Field string
}

func (e *ErrValueTypeUnknown) Error() string {
	return fmt.Sprintf("value type unknown for `%s`", e.Field)
}

// ErrNoApprover is returned when no approver was found for a set of rules
type ErrNoApprover struct {
	Fields []string
}

func (e ErrNoApprover) Error() string {
	return fmt.Sprintf("no approver for fields `%s`", strings.Join(e.Fields, ", "))
}

// ErrNoEventTypeBucket is returned when no bucket could be found for an event type
type ErrNoEventTypeBucket struct {
	EventType string
}

func (e ErrNoEventTypeBucket) Error() string {
	return fmt.Sprintf("no bucket for event type `%s`", e.EventType)
}

// ErrPoliciesLoad is returned on policies dir error
type ErrPoliciesLoad struct {
	Name string
	Err  error
}

func (e ErrPoliciesLoad) Error() string {
	return fmt.Sprintf("policies dir read error `%s`: %s", e.Name, e.Err)
}

// ErrPolicyLoad is returned on policy file error
type ErrPolicyLoad struct {
	Name string
	Err  error
}

func (e ErrPolicyLoad) Error() string {
	return fmt.Sprintf("policy file error `%s`: %s", e.Name, e.Err)
}

// ErrMacroLoad is on macro definition error
type ErrMacroLoad struct {
	Definition *MacroDefinition
	Err        error
}

func (e ErrMacroLoad) Error() string {
	return fmt.Sprintf("macro `%s` definition error: %s", e.Definition.ID, e.Err)
}

// ErrRuleLoad is on rule definition error
type ErrRuleLoad struct {
	Definition *RuleDefinition
	Err        error
}

func (e ErrRuleLoad) Error() string {
	return fmt.Sprintf("rule `%s` error: %s", e.Definition.ID, e.Err)
}

// RuleLoadErrType defines an rule error type
type RuleLoadErrType string

const (
	// AgentVersionErrType agent version incompatible
	AgentVersionErrType RuleLoadErrType = "agent_version_error"
	// AgentFilterErrType agent filter do not match
	AgentFilterErrType RuleLoadErrType = "agent_filter_error"
	// EventTypeNotEnabledErrType event type not enabled
	EventTypeNotEnabledErrType RuleLoadErrType = "event_type_disabled"
	// SyntaxErrType syntax error
	SyntaxErrType RuleLoadErrType = "syntax_error"
	// UnknownErrType undefined error
	UnknownErrType RuleLoadErrType = "error"
)

// Type return the type of the error
func (e ErrRuleLoad) Type() RuleLoadErrType {
	switch e.Err {
	case ErrRuleAgentVersion:
		return AgentVersionErrType
	case ErrRuleAgentFilter:
		return AgentVersionErrType
	case ErrEventTypeNotEnabled:
		return EventTypeNotEnabledErrType
	}

	switch e.Err.(type) {
	case *ErrFieldTypeUnknown, *ErrValueTypeUnknown, *ErrRuleSyntax:
		return SyntaxErrType
	}

	return UnknownErrType
}

// ErrRuleSyntax is returned when there is a syntax error
type ErrRuleSyntax struct {
	Err error
}

func (e *ErrRuleSyntax) Error() string {
	return fmt.Sprintf("syntax error `%v`", e.Err)
}

// ErrActionFilter is on filter definition error
type ErrActionFilter struct {
	Expression string
	Err        error
}

func (e ErrActionFilter) Error() string {
	return fmt.Sprintf("filter `%s` error: %s", e.Expression, e.Err)
}
