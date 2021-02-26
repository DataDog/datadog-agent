// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

// Model - interface that a model has to implement for the rule compilation
type Model interface {
	// GetEvaluator returns an evaluator for the given field
	GetEvaluator(field Field, regID RegisterID) (Evaluator, error)
	// ValidateField returns whether the value use against the field is valid, ex: for constant
	ValidateField(field Field, value FieldValue) error
	// GetIterator return an iterator
	GetIterator(field Field) (Iterator, error)
	// NewEvent returns a new event instance
	NewEvent() Event
	// TranslateLegacyField transforms a legacy attribute into its updated version. Should be idempotent for non legacy
	// fields.
	TranslateLegacyField(field Field) Field
}
