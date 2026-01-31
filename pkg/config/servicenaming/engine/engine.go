// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package engine provides the CEL rule evaluation engine for service naming.
package engine

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"

	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// evalTimeout is the maximum time allowed for CEL expression evaluation
	evalTimeout = 100 * time.Millisecond
)

var (
	// serviceNameRegex validates allowed characters in service names
	// Datadog service names allow alphanumeric, hyphens, underscores, dots, slashes, and colons
	serviceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9._:/\-]+$`)
)

// ServiceDiscoveryResult contains the evaluated service discovery values.
type ServiceDiscoveryResult struct {
	// ServiceName is the computed service name (never empty)
	ServiceName string

	// MatchedRule is the name or index of the rule that matched (for debugging)
	MatchedRule string
}

// CELInput is the input structure for CEL evaluation.
// Fields should be maps generated from servicenaming types.
type CELInput struct {
	Container map[string]any
}

// Rule represents a single CEL rule with query and value expressions.
type Rule struct {
	// Name is an optional identifier for debugging (appears in MatchedRule field).
	// If empty, the rule index will be used instead.
	Name string

	Query string
	Value string
}

// Engine is a CEL rule evaluation engine with precompiled programs.
type Engine struct {
	rules      []compiledRule
	logLimiter *log.Limit
}

// compiledRule holds precompiled CEL programs for a rule.
type compiledRule struct {
	queryProgram cel.Program
	valueProgram cel.Program
	name         string // Optional name for debugging
	index        int    // Rule index (used if name is empty)
}

// NewEngine creates a new CEL rule evaluation engine.
// Returns an error if any rule fails to compile (syntax errors, type mismatches).
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

		// Compile query expression (must be boolean or dyn)
		queryAST, issues := env.Compile(rule.Query)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("rule[%d]: failed to compile query: %w", i, issues.Err())
		}
		// Accept BoolType or DynType (runtime validation will ensure it's actually bool)
		queryType := queryAST.OutputType()
		if queryType != cel.BoolType && queryType != cel.DynType {
			return nil, fmt.Errorf("rule[%d]: query must return boolean, got %v", i, queryType)
		}

		queryProgram, err := env.Program(queryAST)
		if err != nil {
			return nil, fmt.Errorf("rule[%d]: failed to create query program: %w", i, err)
		}

		// Compile value expression (must be string or dyn)
		valueAST, issues := env.Compile(rule.Value)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("rule[%d]: failed to compile value: %w", i, issues.Err())
		}
		// Accept StringType or DynType (runtime validation will ensure it's actually string)
		valueType := valueAST.OutputType()
		if valueType != cel.StringType && valueType != cel.DynType {
			return nil, fmt.Errorf("rule[%d]: value must return string, got %v", i, valueType)
		}

		valueProgram, err := env.Program(valueAST)
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

// Evaluate evaluates the rules against the input in order (first-match-wins).
// Returns the first matching rule's result, or nil if no rule matches.
// Runtime errors are logged (rate-limited) and cause the rule to be skipped.
// Evaluation is bounded by a timeout to prevent resource exhaustion from malicious expressions.
func (e *Engine) Evaluate(ctx context.Context, input CELInput) *ServiceDiscoveryResult {
	if len(e.rules) == 0 {
		return nil
	}

	// Add timeout to prevent resource exhaustion from malicious/complex expressions
	evalCtx, cancel := context.WithTimeout(ctx, evalTimeout)
	defer cancel()

	// Prepare CEL variables (input is already in map format)
	vars := map[string]any{
		"container": input.Container,
	}

	for _, rule := range e.rules {
		ruleID := getRuleID(rule)

		// Check context before evaluating (respect cancellation)
		select {
		case <-evalCtx.Done():
			if e.logLimiter.ShouldLog() {
				log.Warnf("servicenaming: evaluation timeout or cancelled: %v", evalCtx.Err())
			}
			return nil
		default:
		}

		// Evaluate query
		queryResult, _, err := rule.queryProgram.Eval(vars)
		if err != nil {
			if e.logLimiter.ShouldLog() {
				log.Warnf("servicenaming rule[%s]: runtime error evaluating query: %v", ruleID, err)
			}
			continue
		}

		// Check if query result is true
		queryBool, ok := queryResult.Value().(bool)
		if !ok {
			if e.logLimiter.ShouldLog() {
				log.Warnf("servicenaming rule[%s]: query returned non-boolean value: %v", ruleID, queryResult.Value())
			}
			continue
		}

		if !queryBool {
			continue
		}

		// Query matched, evaluate value
		valueResult, _, err := rule.valueProgram.Eval(vars)
		if err != nil {
			if e.logLimiter.ShouldLog() {
				log.Warnf("servicenaming rule[%s]: runtime error evaluating value: %v", ruleID, err)
			}
			continue
		}

		// Extract string value
		valueStr, ok := valueResult.Value().(string)
		if !ok {
			if e.logLimiter.ShouldLog() {
				log.Warnf("servicenaming rule[%s]: value returned non-string value: %v", ruleID, valueResult.Value())
			}
			continue
		}

		// Skip empty service names (likely configuration error or missing data)
		if valueStr == "" {
			if e.logLimiter.ShouldLog() {
				log.Warnf("servicenaming rule[%s]: value evaluated to empty string, skipping", ruleID)
			}
			continue
		}

		// Validate service name format
		if err := validateServiceName(valueStr); err != nil {
			if e.logLimiter.ShouldLog() {
				log.Warnf("servicenaming rule[%s]: invalid service name %q: %v", ruleID, valueStr, err)
			}
			continue
		}

		return &ServiceDiscoveryResult{
			ServiceName: valueStr,
			MatchedRule: ruleID,
		}
	}

	// No rule matched
	return nil
}

// getRuleID returns the rule name if set, otherwise the index as a string.
// This is used for logging and debugging to identify which rule matched or failed.
func getRuleID(rule compiledRule) string {
	if rule.name != "" {
		return rule.name
	}
	return strconv.Itoa(rule.index)
}

// validateServiceName validates that a service name conforms to Datadog's requirements.
// Service names must:
//   - Be 100 characters or less
//   - Contain only alphanumeric characters, hyphens, underscores, dots, slashes, and colons
//   - Not contain leading/trailing whitespace
func validateServiceName(name string) error {
	// Check length using canonical Datadog service name length limit
	if len(name) > normalize.MaxServiceLen {
		return fmt.Errorf("exceeds maximum length of %d characters (got %d)", normalize.MaxServiceLen, len(name))
	}

	// Check for leading/trailing whitespace
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("contains leading or trailing whitespace")
	}

	// Check for invalid characters
	if !serviceNameRegex.MatchString(name) {
		// Find the first invalid character for a helpful error message
		for i, r := range name {
			if !isValidServiceNameChar(r) {
				return fmt.Errorf("contains invalid character %q at position %d", r, i)
			}
		}
		return fmt.Errorf("contains invalid characters")
	}

	return nil
}

// isValidServiceNameChar checks if a rune is valid in a Datadog service name
func isValidServiceNameChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == ':' || r == '/' || r == '-'
}

// CreateCELEnvironment creates and returns the CEL environment for service naming.
// This is exported so it can be reused for validation in the config package.
// The environment declares a "container" variable as DynType for flexibility,
// and enables String and List extensions for common operations.
func CreateCELEnvironment() (*cel.Env, error) {
	return cel.NewEnv(
		// Declare container variable as DynType for flexibility with nil pointers
		cel.Variable("container", cel.DynType),

		// Enable CEL extensions needed for service naming
		ext.Strings(), // String operations: split, startsWith, endsWith, etc.
		ext.Lists(),   // List/map operations: size, exists, map, filter, etc.
	)
}
