// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

// Model - interface that a model has to implement for the rule compilation
type Model interface {
	// GetEvaluator returns an evaluator for the given field
	GetEvaluator(field Field, regID RegisterID, offset int) (Evaluator, error)
	// ValidateField returns whether the value use against the field is valid, ex: for constant
	ValidateField(field Field, value FieldValue) error
	// ValidateRule returns whether the rule is valid
	ValidateRule(_ *Rule) error
	// NewEvent returns a new event instance
	NewEvent() Event
	// GetFieldRestrictions returns the event type for which the field is available
	GetFieldRestrictions(field Field) []EventType
}

// FieldDerivationModel is implemented by the models in which the value of a string field can be computed from the value
// of another one, so that the partial evaluation of a rule on the latter can account for the former
type FieldDerivationModel interface {
	// GetFieldDerivation returns the function computing the value of field from the value of from, nil if there is none
	GetFieldDerivation(field Field, from Field) func(string) string
}
