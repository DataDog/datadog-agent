// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package utils

import (
	"encoding/json"
	"fmt"
	"iter"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetMetadataAsTagExpressions is a version of kubernetesResourcesLabelsAsTags
// which supports expressions on metadata for extraction.
//
// https://docs.google.com/document/d/1P6U1jS2JtaI2G2TRjhiBM2B2SjUDwHXXbU7R4F3MAes/edit?pli=1&tab=t.0
func GetMetadataAsTagExpressions(c pkgconfigmodel.Reader) map[string]ResourceTagExpressions {
	valueFromConfig := c.GetString("kubernetes_resources_metadata_as_tags_expressions")

	var config map[string][]tagSelectionExpressionConfig
	if err := json.Unmarshal([]byte(valueFromConfig), &config); err != nil {
		config = map[string][]tagSelectionExpressionConfig{}
	}

	programs := make(map[string]ResourceTagExpressions, len(config))
	for k, v := range config {
		programs[k] = parseTagExpressionsList(v)
	}

	return programs
}

func parseTagExpressionsList(in []tagSelectionExpressionConfig) []TagExpressions {
	list := make([]TagExpressions, 0, len(in))
	for _, e := range in {
		var expr TagExpressions
		if err := expr.parse(e); err != nil {
			log.Errorf("Error parsing program: %s", err)
			continue
		}

		list = append(list, expr)
	}

	return list
}

type tagSelectionExpressionConfig struct {
	Match string            `json:"match"`
	Tags  map[string]string `json:"tags"`
}

type TagExpressions struct {
	Match TagExpression[bool]
	Tags  map[string]TagExpression[string]
}

func (e *TagExpressions) parse(in tagSelectionExpressionConfig) (err error) {
	if len(in.Tags) == 0 {
		err = fmt.Errorf("no tags specified for expression")
		return
	}

	if in.Match != "" {
		e.Match, err = newExpressionProgram[bool](in.Match)
		if err != nil {
			return
		}
	}

	e.Tags = make(map[string]TagExpression[string], len(in.Tags))
	for tag, expr := range in.Tags {
		e.Tags[tag], err = newExpressionProgram[string](expr)
		if err != nil {
			return
		}
	}

	return
}

func (e *TagExpressions) Eval(meta KubernetesMetadata) iter.Seq2[TagValue, error] {
	return func(yield func(TagValue, error) bool) {
		if e.Match != nil {
			matched, err := e.Match.Eval(meta)
			if err != nil {
				yield(TagValue{}, err)
				return
			} else if !matched {
				return
			}
		}

		for tag, expr := range e.Tags {
			val, err := expr.Eval(meta)
			if err != nil {
				yield(TagValue{}, err)
			} else if val != "" {
				yield(TagValue{Key: tag, Value: val}, nil)
			}
		}
	}
}

type TagValue struct {
	Key   string
	Value string
}

func (t TagValue) IsEmpty() bool {
	return t.Key == "" || t.Value == ""
}

// ResourceTagExpressions ...
type ResourceTagExpressions []TagExpressions

func (r ResourceTagExpressions) Eval(meta KubernetesMetadata) iter.Seq2[TagValue, error] {
	return func(yield func(TagValue, error) bool) {
		for _, expr := range r {
			for kv, err := range expr.Eval(meta) {
				if !yield(kv, err) {
					return
				}
			}
		}
	}
}

// TagExpression is something that can evaluate kubernetes metadata
// to produce a value, or an error, it is generic over its output.
type TagExpression[T any] interface {
	Eval(KubernetesMetadata) (T, error)
}

// KubernetesMetadata represents what the expression has access to.
type KubernetesMetadata struct {
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

func newExpressionProgram[T any](in string) (*expressionProgram[T], error) {
	rt := reflect.TypeOf(&KubernetesMetadata{})
	env, err := cel.NewEnv(
		ext.NativeTypes(rt, ext.ParseStructTag("json")),
		cel.Variable("meta", cel.ObjectType("utils.KubernetesMetadata")),
	)
	if err != nil {
		return nil, fmt.Errorf("error creatring environment: %w", err)
	}

	ast, iss := env.Compile(in)
	if iss != nil {
		if err := iss.Err(); err != nil {
			return nil, fmt.Errorf("compile issues: %w", err)
		}
		return nil, fmt.Errorf("issues compiling program: %s", iss.String())
	}

	// N.B. This is fun!
	outputType := ast.OutputType()
	if valid, err := astConformsToValue[T](outputType); err != nil {
		var empty T
		return nil, fmt.Errorf("expected expression to evaluate to %T type, failed typecheck: %w", empty, err)
	} else if !valid {
		var empty T
		return nil, fmt.Errorf("expected expression to evalute to %T type, got %s instead", empty, outputType)
	}

	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("error creating program from ast: %w", err)
	}

	return &expressionProgram[T]{
		prg: prg,
	}, nil
}

// expressionProgram is a valid, typechecked program that produces
// a typed output.
//
// See [[newExpressionProgram]].
type expressionProgram[T any] struct {
	prg cel.Program
}

func (e *expressionProgram[T]) Eval(meta KubernetesMetadata) (T, error) {
	out, _, err := e.prg.Eval(map[string]any{"meta": &meta})
	if err != nil {
		var empty T
		return empty, fmt.Errorf("error running expression against %+v: %w", meta, err)
	}

	typed, ok := out.Value().(T)
	if !ok {
		var empty T
		return empty, fmt.Errorf("unexpected type %T", out.Value())
	}

	return typed, nil
}

func asAny(in any) any { return in }

func astConformsToValue[T any](ast *cel.Type) (bool, error) {
	var empty T
	switch asAny(empty).(type) {
	case bool:
		return reflect.DeepEqual(ast, cel.BoolType), nil
	case string:
		return reflect.DeepEqual(ast, cel.StringType), nil
	default:
		return false, fmt.Errorf("unsupported type %T", empty)
	}
}
