// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filtermodel holds rules related files
package filtermodel

import (
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// OSOnlyFilterEvent defines an os-only rule filter event
type OSOnlyFilterEvent struct {
	os string
}

// OSOnlyFilterModel defines a filter model
type OSOnlyFilterModel struct {
	os string
}

// NewOSOnlyFilterModel returns a new rule filter model
func NewOSOnlyFilterModel(os string) *OSOnlyFilterModel {
	return &OSOnlyFilterModel{
		os: os,
	}
}

// NewEvent returns a new event
func (m *OSOnlyFilterModel) NewEvent() eval.Event {
	return &OSOnlyFilterEvent{
		os: m.os,
	}
}

// GetEvaluator gets the evaluator
func (m *OSOnlyFilterModel) GetEvaluator(field eval.Field, _ eval.RegisterID, _ int) (eval.Evaluator, error) {
	switch field {
	case "os":
		return &eval.StringEvaluator{
			EvalFnc: func(_ *eval.Context) string { return m.os },
			Field:   field,
		}, nil
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

// GetFieldValue gets a field value
func (e *OSOnlyFilterEvent) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
	case "os":
		return e.os, nil
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

// Init inits the rule filter event
func (e *OSOnlyFilterEvent) Init() {}

// SetFieldValue sets the value for the given field
func (e *OSOnlyFilterEvent) SetFieldValue(field eval.Field, _ interface{}) error {
	return &eval.ErrFieldNotFound{Field: field}
}

// GetFieldMetadata get the type of the field
func (e *OSOnlyFilterEvent) GetFieldMetadata(field eval.Field) (eval.EventType, reflect.Kind, string, bool, error) {
	switch field {
	case "os":
		return "*", reflect.String, "string", false, nil
	}

	return "", reflect.Invalid, "", false, &eval.ErrFieldNotFound{Field: field}
}

// GetType returns the type for this event
func (e *OSOnlyFilterEvent) GetType() string {
	return "*"
}

// GetTags returns the tags for this event
func (e *OSOnlyFilterEvent) GetTags() []string {
	return []string{}
}

// ValidateField returns whether the value use against the field is valid
func (m *OSOnlyFilterModel) ValidateField(_ string, _ eval.FieldValue) error {
	return nil
}

// ValidateRule returns whether the rule is valid
func (m *OSOnlyFilterModel) ValidateRule(_ *eval.Rule) error {
	return nil
}

// GetFieldRestrictions returns the field event type restrictions
func (m *OSOnlyFilterModel) GetFieldRestrictions(_ eval.Field) []eval.EventType {
	return nil
}
