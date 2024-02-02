// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ActionName defines an action name
type ActionName = string

const (
	// KillAction name a the kill action
	KillAction ActionName = "kill"
)

// ActionDefinition describes a rule action section
type ActionDefinition struct {
	Filter *string         `yaml:"filter"`
	Set    *SetDefinition  `yaml:"set"`
	Kill   *KillDefinition `yaml:"kill"`

	// internal
	InternalCallback *InternalCallbackDefinition
	FilterEvaluator  *eval.RuleEvaluator
}

// Check returns an error if the action in invalid
func (a *ActionDefinition) Check() error {
	if a.Set == nil && a.InternalCallback == nil && a.Kill == nil {
		return errors.New("either 'set' or 'kill' section of an action must be specified")
	}

	if a.Set != nil {
		if a.Kill != nil {
			return errors.New("only of 'set' or 'kill' section of an action can be specified")
		}

		if a.Set.Name == "" {
			return errors.New("action name is empty")
		}

		if (a.Set.Value == nil && a.Set.Field == "") || (a.Set.Value != nil && a.Set.Field != "") {
			return errors.New("either 'value' or 'field' must be specified")
		}
	} else if a.Kill != nil {
		if a.Kill.Signal == "" {
			return fmt.Errorf("a valid signal has to be specified to the 'kill' action")
		}

		if _, found := model.SignalConstants[a.Kill.Signal]; !found {
			return fmt.Errorf("unsupported signal '%s'", a.Kill.Signal)
		}
	}

	return nil
}

// CompileFilter compiles the filter expression
func (a *ActionDefinition) CompileFilter(parsingContext *ast.ParsingContext, model eval.Model, evalOpts *eval.Opts) error {
	if a.Filter == nil || *a.Filter == "" {
		return nil
	}

	expression := *a.Filter

	rule := &Rule{
		Rule: eval.NewRule("action_rule", expression, evalOpts),
	}

	if err := rule.Parse(parsingContext); err != nil {
		return &ErrActionFilter{Expression: expression, Err: err}
	}

	if err := rule.GenEvaluator(model, parsingContext); err != nil {
		return &ErrActionFilter{Expression: expression, Err: err}
	}

	a.FilterEvaluator = rule.GetEvaluator()

	return nil
}

// IsAccepted returns whether a filter is accepted and has to be executed
func (a *ActionDefinition) IsAccepted(ctx *eval.Context) bool {
	return a.FilterEvaluator == nil || a.FilterEvaluator.Eval(ctx)
}

// Scope describes the scope variables
type Scope string

// SetDefinition describes the 'set' section of a rule action
type SetDefinition struct {
	Name   string      `yaml:"name"`
	Value  interface{} `yaml:"value"`
	Field  string      `yaml:"field"`
	Append bool        `yaml:"append"`
	Scope  Scope       `yaml:"scope"`
}

// InternalCallbackDefinition describes an internal rule action
type InternalCallbackDefinition struct{}

// KillDefinition describes the 'kill' section of a rule action
type KillDefinition struct {
	Signal string `yaml:"signal"`
	Scope  string `yaml:"scope"`
}
