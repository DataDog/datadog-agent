// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"
	"sort"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// modelTypes serves the generated SECL object types to CEL.
//
// The types are a tree over the SECL field *names*, so `process` is a type with a
// `file` member rather than a few thousand dotted variables. Field types
// therefore reach the checker precisely, and an iterated field such as
// process.ancestors is a list of element objects, which is what lets an iterator
// variable become an exists() over it.
type modelTypes struct {
	types.Provider
}

// FindStructType implements types.Provider.
func (m *modelTypes) FindStructType(name string) (*types.Type, bool) {
	if _, ok := modelShapes[name]; ok {
		return types.NewTypeTypeWithParam(types.NewObjectType(name)), true
	}
	return m.Provider.FindStructType(name)
}

// FindStructFieldNames implements types.Provider. Exposing the member names is
// what makes the schema enumerable, for navigation and for suggesting a
// correction on a misspelt field.
func (m *modelTypes) FindStructFieldNames(name string) ([]string, bool) {
	members, ok := modelShapes[name]
	if !ok {
		return m.Provider.FindStructFieldNames(name)
	}

	names := make([]string, 0, len(members))
	for member := range members {
		names = append(names, member)
	}
	sort.Strings(names)
	return names, true
}

// FindStructFieldType implements types.Provider.
func (m *modelTypes) FindStructFieldType(structType, field string) (*types.FieldType, bool) {
	fieldType, ok := modelShapes[structType][field]
	if !ok {
		return m.Provider.FindStructFieldType(structType, field)
	}

	return &types.FieldType{
		Type:  fieldType,
		IsSet: func(any) bool { return true },
		// The planner turns a select on a known struct type into a call to this
		// getter, which is where a field is read. Nothing is resolved for a member
		// that is itself an object or an iterated field, so an expression only
		// reaches the readers for the leaves it mentions.
		GetFrom: func(obj any) (any, error) {
			object, ok := obj.(*seclObject)
			if !ok {
				return nil, fmt.Errorf("%w: %T is not a SECL event position", errUnsupportedValue, obj)
			}
			return object.selectMember(field, fieldType)
		},
	}, true
}

// ModelTypes returns the environment options that declare the SECL fields: the
// generated object types as a type provider, plus one variable per top level
// segment.
//
// Passing these to NewEnv is what turns Compile from a well-formedness check
// into a real one, so that comparing a string field against an integer, or
// misspelling a field name, is rejected.
func ModelTypes() ([]cel.EnvOption, error) {
	// The registry supplies the standard types the generated ones do not cover.
	registry, err := types.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("creating the CEL type registry: %w", err)
	}

	opts := []cel.EnvOption{cel.CustomTypeProvider(&modelTypes{Provider: registry})}

	roots := make([]string, 0, len(modelRoots))
	for root := range modelRoots {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	for _, root := range roots {
		opts = append(opts, cel.Variable(root, modelRoots[root]))
	}

	return opts, nil
}

// NewModelEnv returns a CEL environment that declares the SECL helper functions
// and every SECL field with its real type.
func NewModelEnv(opts ...cel.EnvOption) (*cel.Env, error) {
	modelOpts, err := ModelTypes()
	if err != nil {
		return nil, err
	}

	env, err := NewEnv(append(modelOpts, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("building the SECL CEL environment: %w", err)
	}
	return env, nil
}
