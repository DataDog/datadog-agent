// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"sort"
)

type registerInfo struct {
	iterator  Iterator
	field     Field
	subFields map[Field]bool
}

type State struct {
	model         Model
	field         Field
	events        map[EventType]bool
	fieldValues   map[Field][]FieldValue
	macros        map[MacroID]*MacroEvaluator
	registersInfo map[RegisterID]*registerInfo
}

func (s *State) UpdateFields(field Field) {
	if _, ok := s.fieldValues[field]; !ok {
		s.fieldValues[field] = []FieldValue{}
	}
}

func (s *State) UpdateFieldValues(field Field, value FieldValue) error {
	values, ok := s.fieldValues[field]
	if !ok {
		values = []FieldValue{}
	}
	values = append(values, value)
	s.fieldValues[field] = values
	return s.model.ValidateField(field, value)
}

func (s *State) Events() []EventType {
	var events []EventType

	for event := range s.events {
		events = append(events, event)
	}
	sort.Strings(events)

	return events
}

func newState(model Model, field Field, macros map[MacroID]*MacroEvaluator) *state {
	if macros == nil {
		macros = make(map[MacroID]*MacroEvaluator)
	}
	return &State{
		field:         field,
		macros:        macros,
		model:         model,
		events:        make(map[EventType]bool),
		fieldValues:   make(map[Field][]FieldValue),
		registersInfo: make(map[RegisterID]*registerInfo),
	}
}
