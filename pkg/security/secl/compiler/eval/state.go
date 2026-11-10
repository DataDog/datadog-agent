// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

// State defines the current state of the rule compilation
type State struct {
	model       Model
	field       Field
	fieldValues map[Field][]FieldValue
	registers   []Register

	// referencedFields holds all the fields resolved while compiling, including the ones
	// used through field references (%{field}) or through macros
	referencedFields map[Field]bool
	// referencedVariables holds all the variables resolved while compiling, including the
	// ones used through macros
	referencedVariables map[string]bool
}

// GetReferencedFields returns all the fields resolved while compiling
func (s *State) GetReferencedFields() []Field {
	return KeysOfMap(s.referencedFields)
}

// GetReferencedVariables returns all the variables resolved while compiling
func (s *State) GetReferencedVariables() []string {
	return KeysOfMap(s.referencedVariables)
}

// getEvaluator returns the evaluator of a field, and records the field as referenced
func (s *State) getEvaluator(field Field, regID RegisterID, offset int) (Evaluator, error) {
	evaluator, err := s.model.GetEvaluator(field, regID, offset)
	if err == nil {
		s.referencedFields[field] = true
	}
	return evaluator, err
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
func NewState(model Model, field Field) *State {
	return &State{
		field:       field,
		model:       model,
		fieldValues: make(map[Field][]FieldValue),

		referencedFields:    make(map[Field]bool),
		referencedVariables: make(map[string]bool),
	}
}
