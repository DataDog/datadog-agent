// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package eval

// Model - interface that a model has to implement for the rule compilation
type Model interface {
	// GetEvaluator - Returns an evaluator for the given field
	GetEvaluator(field Field) (interface{}, error)
	// ValidateField - Returns whether the value use against the field is valid, ex: for constant
	ValidateField(field Field, value FieldValue) error
	// NewEvent - Returns a new event instance
	NewEvent() Event
}
