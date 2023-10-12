// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

// MacroStore represents a store of SECL Macros
type MacroStore struct {
	Macros map[MacroID]*Macro
}

// Add adds a macro
func (s *MacroStore) Add(macro *Macro) *MacroStore {
	if s.Macros == nil {
		s.Macros = make(map[string]*Macro)
	}
	s.Macros[macro.ID] = macro
	return s
}

// List lists macros
func (s *MacroStore) List() []*Macro {
	var macros []*Macro

	if s == nil || s.Macros == nil {
		return macros
	}

	for _, macro := range s.Macros {
		macros = append(macros, macro)
	}
	return macros
}

// Get returns the marcro
func (s *MacroStore) Get(id string) *Macro {
	if s == nil || s.Macros == nil {
		return nil
	}
	return s.Macros[id]
}

// VariableStore represents a store of SECL variables
type VariableStore struct {
	Variables map[string]VariableValue
}

// Add adds a variable
func (s *VariableStore) Add(name string, variable VariableValue) *VariableStore {
	if s.Variables == nil {
		s.Variables = make(map[string]VariableValue)
	}
	s.Variables[name] = variable
	return s
}

// Get returns the variable
func (s *VariableStore) Get(name string) VariableValue {
	if s == nil || s.Variables == nil {
		return nil
	}
	return s.Variables[name]
}

// Opts are the options to be passed to the evaluator
type Opts struct {
	LegacyFields  map[Field]Field
	Constants     map[string]interface{}
	VariableStore *VariableStore
	MacroStore    *MacroStore
}

// WithConstants set constants
func (o *Opts) WithConstants(constants map[string]interface{}) *Opts {
	o.Constants = constants
	return o
}

// WithVariables set variables
func (o *Opts) WithVariables(variables map[string]VariableValue) *Opts {
	if o.VariableStore == nil {
		o.VariableStore = &VariableStore{}
	}

	for n, v := range variables {
		o.VariableStore.Add(n, v)
	}
	return o
}

// WithVariableStore set the variable store
func (o *Opts) WithVariableStore(store *VariableStore) *Opts {
	o.VariableStore = store
	return o
}

// WithLegacyFields set legacy fields
func (o *Opts) WithLegacyFields(fields map[Field]Field) *Opts {
	o.LegacyFields = fields
	return o
}

// WithMacroStore set the macro store
func (o *Opts) WithMacroStore(store *MacroStore) *Opts {
	o.MacroStore = store
	return o
}

// AddMacro add a macro
func (o *Opts) AddMacro(macro *Macro) *Opts {
	if o.MacroStore == nil {
		o.MacroStore = &MacroStore{}
	}
	o.MacroStore.Add(macro)
	return o
}

// AddVariable add a variable
func (o *Opts) AddVariable(name string, variable VariableValue) *Opts {
	if o.VariableStore == nil {
		o.VariableStore = &VariableStore{}
	}
	o.VariableStore.Add(name, variable)
	return o
}
