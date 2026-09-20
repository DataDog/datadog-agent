// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"fmt"
	"maps"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
)

const (
	// celCostLimit bounds the evaluation cost of a single expression.
	celCostLimit = 1_000_000

	varEntity = "entity"
	varSource = "source"
)

// Evaluator compiles and caches CEL programs for tag-rule expressions.
// Programs are safe for concurrent use.
type Evaluator struct {
	env *cel.Env

	mu       sync.Mutex
	programs map[string]cel.Program
}

// NewEvaluator builds the CEL environment for tag-rule expressions. `entity`
// and `source` are unstructured object maps; identifier-safe map keys use dot
// access, keys with dots require bracket access.
func NewEvaluator() (*Evaluator, error) {
	env, err := cel.NewEnv(
		cel.Variable(varEntity, cel.DynType),
		cel.Variable(varSource, cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("building CEL environment: %w", err)
	}
	return &Evaluator{env: env, programs: map[string]cel.Program{}}, nil
}

// program compiles (once) and returns the program for an expression. outType
// is the required result type; a mismatch is a rule configuration error.
func (e *Evaluator) program(expression string, outType *cel.Type) (cel.Program, error) {
	cacheKey := fmt.Sprintf("%s:%s", outType.String(), expression)

	e.mu.Lock()
	defer e.mu.Unlock()
	if program, ok := e.programs[cacheKey]; ok {
		return program, nil
	}

	ast, issues := e.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compiling expression %q: %w", expression, issues.Err())
	}
	// Output types are checked at runtime: operands are dyn maps, so the
	// static output type of most expressions is dyn rather than the
	// primitive the expression yields.
	program, err := e.env.Program(ast, cel.EvalOptions(cel.OptTrackCost), cel.CostLimit(celCostLimit))
	if err != nil {
		return nil, fmt.Errorf("building program for expression %q: %w", expression, err)
	}
	e.programs[cacheKey] = program
	return program, nil
}

// normalizeForEval returns a shallow copy of an object map with
// metadata.labels and metadata.annotations defaulted to empty maps, so CEL
// presence tests ('key in entity.metadata.labels') behave on objects without
// labels instead of erroring. Deeper nested maps are not normalized.
func normalizeForEval(object map[string]any) map[string]any {
	normalized := maps.Clone(object)
	if normalized == nil {
		return map[string]any{}
	}
	metadataAny, ok := normalized["metadata"].(map[string]any)
	if !ok {
		return normalized
	}
	metadata := maps.Clone(metadataAny)
	if _, ok := metadata["labels"].(map[string]any); !ok {
		metadata["labels"] = map[string]any{}
	}
	if _, ok := metadata["annotations"].(map[string]any); !ok {
		metadata["annotations"] = map[string]any{}
	}
	normalized["metadata"] = metadata
	return normalized
}

func (e *Evaluator) eval(expression string, outType *cel.Type, entity, source map[string]any) (ref.Val, error) {
	program, err := e.program(expression, outType)
	if err != nil {
		return nil, err
	}
	out, _, err := program.Eval(map[string]any{varEntity: entity, varSource: source})
	if err != nil {
		return nil, fmt.Errorf("evaluating expression %q: %w", expression, err)
	}
	return out, nil
}

// EvalBool evaluates a boolean expression over entity and source.
func (e *Evaluator) EvalBool(expression string, entity, source map[string]any) (bool, error) {
	out, err := e.eval(expression, cel.BoolType, entity, source)
	if err != nil {
		return false, err
	}
	result, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expression %q returned %T instead of bool", expression, out.Value())
	}
	return result, nil
}

// EvalString evaluates a string expression over entity and source.
func (e *Evaluator) EvalString(expression string, entity, source map[string]any) (string, error) {
	out, err := e.eval(expression, cel.StringType, entity, source)
	if err != nil {
		return "", err
	}
	result, ok := out.Value().(string)
	if !ok {
		return "", fmt.Errorf("expression %q returned %T instead of string", expression, out.Value())
	}
	return result, nil
}
