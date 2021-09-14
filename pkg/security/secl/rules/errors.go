// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

var (
	// ErrRuleWithoutEvent is returned when no event type was inferred from the rule
	ErrRuleWithoutEvent = errors.New("no event in the rule definition")

	// ErrRuleWithMultipleEvents is returned when multiple event type were inferred from the rule
	ErrRuleWithMultipleEvents = errors.New("rule with multiple events is not supported")

	// ErrDefinitionIDConflict is returned when mlultiple rule use the same ID
	ErrDefinitionIDConflict = errors.New("multiple definition with the same ID")

	// ErrInternalIDConflict is returned when a user defined rule use an internal ID
	ErrInternalIDConflict = errors.New("internal rule ID conflict")

	// ErrEventTypeNotEnabled is returned when an event is not enabled
	ErrEventTypeNotEnabled = errors.New("event type not enabled")
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

// ErrProfileLoad is returned on profile file error
type ErrProfileLoad struct {
	Name string
	Err  error
}

func (e ErrProfileLoad) Error() string {
	return fmt.Sprintf("profile file error `%s`: %s", e.Name, e.Err)
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
	return fmt.Sprintf("rule `%s` definition error: %s", e.Definition.ID, e.Err)
}
