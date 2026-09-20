// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagruleswebhook

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/google/cel-go/cel"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/tagrules"
)

const (
	// tagKeyPattern is the tag-key syntax accepted for spec.tag.
	tagKeyPattern = `^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`
	// tagKeyMaxLen is the maximum spec.tag length.
	tagKeyMaxLen = 100
	// flipHoldFloor mirrors the CRD's spec.flipHoldSeconds minimum.
	flipHoldFloor = int64(tagrules.DefaultFlipHold)
	// celCostLimit mirrors the controller's evaluation cost limit.
	celCostLimit = 1_000_000

	// varEntity and varSource mirror the controller's CEL variables.
	varEntity = "entity"
	varSource = "source"
)

var tagKeyRegexp = regexp.MustCompile(tagKeyPattern)

// celCompiler compile-checks tag-rule expressions with the same environment
// the controller evaluates them in: entity and source as dyn maps.
type celCompiler struct {
	env *cel.Env
}

func newCelCompiler() (*celCompiler, error) {
	env, err := cel.NewEnv(
		cel.Variable(varEntity, cel.DynType),
		cel.Variable(varSource, cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("building CEL environment: %w", err)
	}
	return &celCompiler{env: env}, nil
}

// compile reports whether the expression compiles and stays within the cost
// budget. No static output type is required: operands are dyn maps, so most
// expressions statically evaluate to dyn.
func (c *celCompiler) compile(expression string) error {
	ast, issues := c.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("compiling expression %q: %w", expression, issues.Err())
	}
	_, err := c.env.Program(ast, cel.EvalOptions(cel.OptTrackCost), cel.CostLimit(celCostLimit))
	if err != nil {
		return fmt.Errorf("building program for expression %q: %w", expression, err)
	}
	return nil
}

// validateRule returns the list of structural problems with a TagRule. It
// mirrors the TagRule CRD's x-kubernetes-validations and the controller's
// validateRule invariants, plus the checks the schema cannot express
// (duplicate string-set values, CEL compilation).
func validateRule(rule *tagrules.TagRule, compiler *celCompiler) []string {
	var problems []string
	problemf := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	switch rule.Spec.Entity {
	case tagrules.EntityPod, tagrules.EntityNode:
	default:
		problemf("unsupported entity kind %q", rule.Spec.Entity)
	}
	if rule.Spec.Entity == tagrules.EntityNode && rule.Spec.Selector.Namespace != "" {
		problemf("namespace selector is not supported for node rules")
	}

	if rule.Spec.Tag == "" {
		problemf("spec.tag must not be empty")
	} else {
		if len(rule.Spec.Tag) > tagKeyMaxLen {
			problemf("spec.tag must be at most %d characters", tagKeyMaxLen)
		}
		if !tagKeyRegexp.MatchString(rule.Spec.Tag) {
			problemf("spec.tag %q must match %s", rule.Spec.Tag, tagKeyPattern)
		}
	}

	if rule.Spec.FlipHoldSeconds != nil && *rule.Spec.FlipHoldSeconds < flipHoldFloor {
		problemf("spec.flipHoldSeconds must be at least %d", flipHoldFloor)
	}

	switch rule.Spec.Value.Type {
	case tagrules.ValueBool:
		if rule.Spec.Value.Bool == nil || rule.Spec.Value.Bool.Expression == "" {
			problemf("type bool requires value.bool.expression")
		}
		if rule.Spec.Value.StringSet != nil {
			problemf("type bool must not set value.stringSet")
		}
	case tagrules.ValueStringSet:
		ss := rule.Spec.Value.StringSet
		if ss == nil || len(ss.Values) == 0 || ss.Expression == "" {
			problemf("type string_set requires value.stringSet.values and value.stringSet.expression")
		}
		if rule.Spec.Value.Bool != nil {
			problemf("type string_set must not set value.bool")
		}
		if ss != nil {
			switch ss.OnError {
			case tagrules.OnErrorDrop, tagrules.OnErrorDefault, "":
			default:
				problemf("unsupported onError action %q", ss.OnError)
			}
			if ss.OnError == tagrules.OnErrorDefault {
				if !slices.Contains(ss.Values, ss.Default) {
					problemf("value.stringSet.default %q must be a member of values", ss.Default)
				}
			}
			for i, value := range ss.Values {
				if slices.Contains(ss.Values[i+1:], value) {
					problemf("value.stringSet.values contains duplicate entry %q", value)
					break
				}
			}
		}
	default:
		problemf("unsupported value type %q", rule.Spec.Value.Type)
	}

	// CEL compile checks for every expression the controller evaluates.
	if rule.Spec.Value.Bool != nil && rule.Spec.Value.Bool.Expression != "" {
		if err := compiler.compile(rule.Spec.Value.Bool.Expression); err != nil {
			problemf("value.bool.expression: %v", err)
		}
	}
	if rule.Spec.Value.StringSet != nil && rule.Spec.Value.StringSet.Expression != "" {
		if err := compiler.compile(rule.Spec.Value.StringSet.Expression); err != nil {
			problemf("value.stringSet.expression: %v", err)
		}
	}
	if rule.Spec.Source != nil {
		if rule.Spec.Source.Kind == "" {
			problemf("source kind must not be empty")
		}
		if rule.Spec.Source.Name != "" {
			if err := compiler.compile(rule.Spec.Source.Name); err != nil {
				problemf("source.name: %v", err)
			}
		}
		if rule.Spec.Source.Namespace != "" {
			if err := compiler.compile(rule.Spec.Source.Namespace); err != nil {
				problemf("source.namespace: %v", err)
			}
		}
	}

	return problems
}

// validateOwnership enforces spec.tag immutability across updates and global
// tag-key uniqueness. claimed maps CR name -> spec.tag for the other TagRules
// in the cluster; the rule's own name is excluded.
func validateOwnership(rule *tagrules.TagRule, old *tagrules.TagRule, claimed map[string]string) []string {
	var problems []string

	if old != nil && old.Spec.Tag != rule.Spec.Tag {
		problems = append(problems, fmt.Sprintf("spec.tag is immutable (was %q, now %q)", old.Spec.Tag, rule.Spec.Tag))
	}

	for name, tag := range claimed {
		if name != rule.Name && tag == rule.Spec.Tag {
			problems = append(problems, fmt.Sprintf("tag key %q is already owned by rule %s", rule.Spec.Tag, name))
		}
	}

	return problems
}
