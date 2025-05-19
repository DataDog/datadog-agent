// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Action represents the action to take when a rule is triggered
// It can either come from policy a definition or be an internal callback
type Action struct {
	Def              *ActionDefinition
	InternalCallback *InternalCallbackDefinition
	FilterEvaluator  *eval.RuleEvaluator
}

// Check returns an error if the action in invalid
func (a *ActionDefinition) Check(opts PolicyLoaderOpts) error {
	if a.Set == nil && a.Kill == nil && a.Hash == nil && a.CoreDump == nil && a.Log == nil {
		return errors.New("either 'set', 'kill', 'hash', 'coredump' or 'log' section of an action must be specified")
	}

	if a.Set != nil {
		if a.Kill != nil || a.Hash != nil || a.CoreDump != nil || a.Log != nil {
			return errors.New("only of 'set' or 'kill' section of an action can be specified")
		}

		if a.Set.Name == "" {
			return errors.New("variable name is empty")
		}

		if a.Set.DefaultValue != nil {
			if defaultValueType, valueType := reflect.TypeOf(a.Set.DefaultValue), reflect.TypeOf(a.Set.Value); valueType != nil && defaultValueType != valueType {
				return fmt.Errorf("'default_value' and 'value' must be of the same type (%s != %s)", defaultValueType, valueType)
			}
		}

		if (a.Set.Value == nil && a.Set.Expression == "" && a.Set.Field == "") ||
			(a.Set.Expression != "" && a.Set.Field != "") ||
			(a.Set.Field != "" && a.Set.Value != nil) ||
			(a.Set.Value != nil && a.Set.Expression != "") {
			return errors.New("either 'value', 'field' or 'expression' must be specified")
		}
	} else if a.Kill != nil {
		if opts.DisableEnforcement {
			a.Kill = nil
			return errors.New("'kill' action is disabled globally")
		}

		if a.Kill.Signal == "" {
			return errors.New("a valid signal has to be specified to the 'kill' action")
		}

		if _, found := model.SignalConstants[a.Kill.Signal]; !found {
			return fmt.Errorf("unsupported signal '%s'", a.Kill.Signal)
		}
	} else if a.Log != nil {
		if a.Log.Level == "" {
			return errors.New("a valid log level must be specified to the the 'log' action")
		}
	}

	return nil
}

// CompileFilter compiles the filter expression
func (a *Action) CompileFilter(parsingContext *ast.ParsingContext, model eval.Model, evalOpts *eval.Opts) error {
	if a.Def.Filter == nil || *a.Def.Filter == "" {
		return nil
	}

	expression := *a.Def.Filter

	eval, err := eval.NewRule("action_rule", expression, parsingContext, evalOpts)
	if err != nil {
		return &ErrActionFilter{Expression: expression, Err: err}
	}

	if err := eval.GenEvaluator(model); err != nil {
		return &ErrActionFilter{Expression: expression, Err: err}
	}

	a.FilterEvaluator = eval.GetEvaluator()

	return nil
}

// IsAccepted returns whether a filter is accepted and has to be executed
func (a *Action) IsAccepted(ctx *eval.Context) bool {
	return a.FilterEvaluator == nil || a.FilterEvaluator.Eval(ctx)
}

// InternalCallbackDefinition describes an internal rule action
type InternalCallbackDefinition struct{}
