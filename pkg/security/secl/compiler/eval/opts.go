// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

type MacroStore struct {
	Macros map[MacroID]*Macro
}

// WithMacros set macros fields
func (s *MacroStore) WithMacros(macros map[MacroID]*Macro) *MacroStore {
	s.Macros = macros
	return s
}

// AddMacro add a macro
func (s *MacroStore) AddMacro(macro *Macro) *MacroStore {
	if s.Macros == nil {
		s.Macros = make(map[string]*Macro)
	}
	s.Macros[macro.ID] = macro
	return s
}

// Opts are the options to be passed to the evaluator
type Opts struct {
	LegacyFields map[Field]Field
	Constants    map[string]interface{}
	Variables    map[string]VariableValue
}

// WithConstants set constants
func (o *Opts) WithConstants(constants map[string]interface{}) *Opts {
	o.Constants = constants
	return o
}

// WithVariables set variables
func (o *Opts) WithVariables(variables map[string]VariableValue) *Opts {
	optsVariables := make(map[string]VariableValue, len(variables))
	for name, value := range variables {
		optsVariables[name] = value
	}

	o.Variables = optsVariables
	return o
}

// WithLegacyFields set legacy fields
func (o *Opts) WithLegacyFields(fields map[Field]Field) *Opts {
	o.LegacyFields = fields
	return o
}
