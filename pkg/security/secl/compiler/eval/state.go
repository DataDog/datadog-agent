// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

type registerInfo struct {
	iterator  Iterator
	field     Field
	subFields map[Field]bool
}

// RuleState defines the current state of the rule compilation
type RuleState struct {
	model         Model
	field         Field
	events        map[EventType]bool
	fieldValues   map[Field][]FieldValue
	macros        map[MacroID]*MacroEvaluator
	registersInfo map[RegisterID]*registerInfo
}

// UpdateFields updates the fields used in the rule
func (s *RuleState) UpdateFields(field Field) {
	if _, ok := s.fieldValues[field]; !ok {
		s.fieldValues[field] = []FieldValue{}
	}
}

// UpdateFieldValues updates the field values
func (s *RuleState) UpdateFieldValues(field Field, value FieldValue) error {
	values, ok := s.fieldValues[field]
	if !ok {
		values = []FieldValue{}
	}
	values = append(values, value)
	s.fieldValues[field] = values
	return s.model.ValidateField(field, value)
}

// GetFields returns all the Field that the state handles
func (s *RuleState) GetFields() []Field {
	fields := make([]Field, len(s.fieldValues))
	i := 0
	for key := range s.fieldValues {
		fields[i] = key
		i++
	}
	return fields
}

// GetFieldValues returns the values of the given field
func (s *RuleState) GetFieldValues(field Field) []FieldValue {
	return s.fieldValues[field]
}

func newRuleState(model Model, field Field, macros map[MacroID]*MacroEvaluator) *RuleState {
	if macros == nil {
		macros = make(map[MacroID]*MacroEvaluator)
	}
	return &RuleState{
		field:         field,
		macros:        macros,
		model:         model,
		events:        make(map[EventType]bool),
		fieldValues:   make(map[Field][]FieldValue),
		registersInfo: make(map[RegisterID]*registerInfo),
	}
}
