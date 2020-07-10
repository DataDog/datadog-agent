package eval

import "sort"

type state struct {
	model       Model
	field       Field
	events      map[EventType]bool
	fieldValues map[Field][]FieldValue
	macros      map[MacroID]*MacroEvaluator
}

//
func (s *state) UpdateFields(field Field) {
	if _, ok := s.fieldValues[field]; !ok {
		s.fieldValues[field] = []FieldValue{}
	}
}

func (s *state) UpdateFieldValues(field Field, value FieldValue) error {
	values, ok := s.fieldValues[field]
	if !ok {
		values = []FieldValue{}
	}
	values = append(values, value)
	s.fieldValues[field] = values
	return s.model.ValidateField(field, value)
}

func (s *state) Events() []EventType {
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
	return &state{
		field:       field,
		macros:      macros,
		model:       model,
		events:      make(map[EventType]bool),
		fieldValues: make(map[Field][]FieldValue),
	}
}
