// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

// Opts are the options to be passed to the evaluator
type Opts struct {
	LegacyFields map[Field]Field
	Constants    map[string]interface{}
	Macros       map[MacroID]*Macro
	Variables    map[string]VariableValue
	UserCtx      interface{}
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

// WithMacros set macros fields
func (o *Opts) WithMacros(macros map[MacroID]*Macro) *Opts {
	o.Macros = macros
	return o
}

// AddMacro add a macro
func (o *Opts) AddMacro(macro *Macro) *Opts {
	if o.Macros == nil {
		o.Macros = make(map[string]*Macro)
	}
	o.Macros[macro.ID] = macro
	return o
}

// WithUserContext set user context
func (o *Opts) WithUserContext(ctx interface{}) *Opts {
	o.UserCtx = ctx
	return o
}
