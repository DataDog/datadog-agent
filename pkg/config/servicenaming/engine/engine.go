// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package engine provides the CEL rule evaluation engine for service naming.
package engine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// evalTimeout is the maximum time allowed for CEL expression evaluation
	evalTimeout = 100 * time.Millisecond

	// maxServiceNameLen is the maximum allowed length for service names.
	// This matches Datadog's service name length limit enforced by the backend platform.
	// The 100-character limit is consistent across all Datadog products (APM, metrics, logs, RUM)
	// to ensure service names are properly indexed and displayed in the UI.
	//
	// DO NOT change this value without confirming that:
	// 1. The Datadog backend supports longer service names
	// 2. All downstream consumers (UI, alerting, etc.) handle longer names
	// 3. The change is coordinated across all agent features that use service names
	maxServiceNameLen = 100
)

// ServiceDiscoveryResult contains the result of CEL rule evaluation.
type ServiceDiscoveryResult struct {
	ServiceName string
	MatchedRule string // Rule name or index for debugging
}

// CELInput is the input for CEL evaluation (Container must be a map).
type CELInput struct {
	Container map[string]any
}

// Rule represents a CEL rule with query (boolean) and value (string) expressions.
type Rule struct {
	Name  string // Optional name for debugging; index used if empty
	Query string
	Value string
}

// Engine evaluates CEL rules with precompiled programs.
type Engine struct {
	rules      []compiledRule
	logLimiter *log.Limit
}

type compiledRule struct {
	queryProgram cel.Program
	valueProgram cel.Program
	name         string
	index        int
}

// NewEngine creates a CEL evaluation engine from rules.
func NewEngine(rules []Rule) (*Engine, error) {
	if len(rules) == 0 {
		return &Engine{rules: []compiledRule{}}, nil
	}

	env, err := CreateCELEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	compiled := make([]compiledRule, 0, len(rules))
	for i, rule := range rules {
		if rule.Query == "" {
			return nil, fmt.Errorf("rule[%d]: query cannot be empty", i)
		}
		if rule.Value == "" {
			return nil, fmt.Errorf("rule[%d]: value cannot be empty", i)
		}

		queryAST, issues := env.Compile(rule.Query)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("rule[%d]: failed to compile query: %w", i, issues.Err())
		}
		queryType := queryAST.OutputType()
		if queryType != cel.BoolType && queryType != cel.DynType {
			return nil, fmt.Errorf("rule[%d]: query must return boolean, got %v", i, queryType)
		}

		queryProgram, err := env.Program(queryAST, cel.CostLimit(10000))
		if err != nil {
			return nil, fmt.Errorf("rule[%d]: failed to create query program: %w", i, err)
		}

		valueAST, issues := env.Compile(rule.Value)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("rule[%d]: failed to compile value: %w", i, issues.Err())
		}
		valueType := valueAST.OutputType()
		if valueType != cel.StringType && valueType != cel.DynType {
			return nil, fmt.Errorf("rule[%d]: value must return string, got %v", i, valueType)
		}

		valueProgram, err := env.Program(valueAST, cel.CostLimit(10000))
		if err != nil {
			return nil, fmt.Errorf("rule[%d]: failed to create value program: %w", i, err)
		}

		compiled = append(compiled, compiledRule{
			queryProgram: queryProgram,
			valueProgram: valueProgram,
			name:         rule.Name,
			index:        i,
		})
	}

	return &Engine{
		rules:      compiled,
		logLimiter: log.NewLogLimit(20, 10*time.Minute),
	}, nil
}

// Evaluate evaluates rules in order (first-match-wins) and returns the matching result.
// Runtime errors are logged (rate-limited) and skip the failing rule.
func (e *Engine) Evaluate(ctx context.Context, input CELInput) *ServiceDiscoveryResult {
	if len(e.rules) == 0 {
		return nil
	}

	vars := map[string]any{"container": input.Container}

	for _, rule := range e.rules {
		// Evaluate each rule with its own timeout context to ensure fair budget distribution
		result := e.evaluateRule(ctx, rule, vars)
		if result != nil {
			return result
		}
	}

	return nil
}

// evaluateRule evaluates a single rule with a dedicated timeout context.
// Returns nil if the rule doesn't match or encounters an error.
func (e *Engine) evaluateRule(ctx context.Context, rule compiledRule, vars map[string]any) *ServiceDiscoveryResult {
	ruleID := getRuleID(rule)

	// Create per-rule timeout context to ensure each rule gets the full timeout budget.
	// This prevents later rules from being starved if earlier rules take time to evaluate.
	evalCtx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()

	select {
	case <-evalCtx.Done():
		if e.logLimiter.ShouldLog() {
			log.Warnf("servicenaming: evaluation timeout or cancelled: %v", evalCtx.Err())
		}
		return nil
	default:
	}

	queryResult, _, err := rule.queryProgram.Eval(vars)
	if err != nil {
		if e.logLimiter.ShouldLog() {
			log.Warnf("servicenaming rule[%s]: runtime error evaluating query: %v", ruleID, err)
		}
		return nil
	}

	queryBool, ok := queryResult.Value().(bool)
	if !ok {
		if e.logLimiter.ShouldLog() {
			log.Warnf("servicenaming rule[%s]: query returned non-boolean value: %v", ruleID, queryResult.Value())
		}
		return nil
	}

	if !queryBool {
		return nil
	}

	valueResult, _, err := rule.valueProgram.Eval(vars)
	if err != nil {
		if e.logLimiter.ShouldLog() {
			log.Warnf("servicenaming rule[%s]: runtime error evaluating value: %v", ruleID, err)
		}
		return nil
	}

	valueStr, ok := valueResult.Value().(string)
	if !ok {
		if e.logLimiter.ShouldLog() {
			log.Warnf("servicenaming rule[%s]: value returned non-string value: %v", ruleID, valueResult.Value())
		}
		return nil
	}

	if valueStr == "" {
		if e.logLimiter.ShouldLog() {
			log.Warnf("servicenaming rule[%s]: value evaluated to empty string, skipping", ruleID)
		}
		return nil
	}

	if err := validateServiceName(valueStr); err != nil {
		if e.logLimiter.ShouldLog() {
			log.Warnf("servicenaming rule[%s]: invalid service name %q: %v", ruleID, valueStr, err)
		}
		return nil
	}

	return &ServiceDiscoveryResult{
		ServiceName: valueStr,
		MatchedRule: ruleID,
	}
}

func getRuleID(rule compiledRule) string {
	if rule.name != "" {
		return rule.name
	}
	return strconv.Itoa(rule.index)
}

// validateServiceName checks that service names meet Datadog requirements:
// max 100 chars, alphanumeric plus [-_./:], no leading/trailing whitespace.
func validateServiceName(name string) error {
	if len(name) > maxServiceNameLen {
		return fmt.Errorf("exceeds maximum length of %d characters (got %d)", maxServiceNameLen, len(name))
	}

	if strings.TrimSpace(name) != name {
		return errors.New("contains leading or trailing whitespace")
	}

	for i, r := range name {
		if !isValidServiceNameChar(r) {
			return fmt.Errorf("contains invalid character %q at position %d (allowed: alphanumeric, -, _, ., /, :)", r, i)
		}
	}

	return nil
}

func isValidServiceNameChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
		r == '.' || r == '_' || r == ':' || r == '/' || r == '-'
}

// CreateCELEnvironment creates the CEL environment for service naming with DynType container variable.
func CreateCELEnvironment() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("container", cel.DynType),
		ext.Strings(),
		ext.Lists(),
	)
}
