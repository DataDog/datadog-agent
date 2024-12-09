// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"regexp"
)

// StateRegexpCache is used to cache regexps used in the rule compilation process
type StateRegexpCache struct {
	arraySubscriptFindRE    *regexp.Regexp
	arraySubscriptReplaceRE *regexp.Regexp
}

// State defines the current state of the rule compilation
type State struct {
	model       Model
	field       Field
	fieldValues map[Field][]FieldValue
	macros      map[MacroID]*MacroEvaluator
	regexpCache StateRegexpCache
	registers   []Register
}

// UpdateFields updates the fields used in the rule
func (s *State) UpdateFields(field Field) {
	if _, ok := s.fieldValues[field]; !ok {
		s.fieldValues[field] = []FieldValue{}
	}
}

// UpdateFieldValues updates the field values
func (s *State) UpdateFieldValues(field Field, value FieldValue) error {
	values := s.fieldValues[field]
	for _, v := range values {
		// compare only comparable
		switch v.Value.(type) {
		case int, uint, int64, uint64, string, bool:
			if v == value {
				return nil
			}
		}
	}

	values = append(values, value)
	s.fieldValues[field] = values
	return s.model.ValidateField(field, value)
}

// NewState returns a new State
func NewState(model Model, field Field, macros map[MacroID]*MacroEvaluator) *State {
	if macros == nil {
		macros = make(map[MacroID]*MacroEvaluator)
	}
	return &State{
		field:       field,
		macros:      macros,
		model:       model,
		fieldValues: make(map[Field][]FieldValue),
	}
}
